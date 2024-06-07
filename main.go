package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	ipfspath "github.com/ipfs/boxo/path"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/kubo/client/rpc"
	"github.com/multiformats/go-multibase"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "ipfs-endpoint",
				Value: "http://127.0.0.1:5001",
				Usage: "IPFS kubo endpoint",
			},
			&cli.StringFlag{
				Name:  "l1-rpc",
				Value: "https://ethereum-rpc.publicnode.com",
				Usage: "L1 RPC url",
			},
			&cli.StringFlag{
				Name:  "batcher-address",
				Value: "0x6017f75108f251a488B045A7ce2a7C15b179d1f2",
				Usage: "Batcher address",
			},
			&cli.StringFlag{
				Name:  "batcher-inbox",
				Value: "0xfF000000000000000000000000000000000420fC",
				Usage: "Batcher inbox address",
			},
			&cli.UintFlag{
				Name:  "start-block",
				Value: 19135636,
				Usage: "Starting block (L1 block of L2 gneesis)",
			},
			&cli.BoolFlag{
				Name:  "debug",
				Usage: "Enabled debug logging",
			},
			&cli.StringFlag{
				Name:  "last-block-path",
				Value: "./last-block",
				Usage: "A path to a file to store the last processed block",
			},
		},
		Action: func(cCtx *cli.Context) error {
			if cCtx.Bool("debug") {
				slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
					Level: slog.LevelDebug,
				})))
			}

			return start(cCtx.Context, config{
				l1Rpc:          cCtx.String("l1-rpc"),
				ipfsEndpoint:   cCtx.String("ipfs-endpoint"),
				batcherAddress: cCtx.String("batcher-address"),
				batcherInbox:   cCtx.String("batcher-inbox"),
				startBlock:     cCtx.Uint("start-block"),
				lastBlockPath:  cCtx.String("last-block-path"),
			})

		},
	}

	if err := app.Run(os.Args); err != nil {
		slog.Error("Application failed", "err", err)
		os.Exit(1)
	}
}

type config struct {
	l1Rpc          string
	ipfsEndpoint   string
	batcherAddress string
	batcherInbox   string
	startBlock     uint
	lastBlockPath  string
}

func start(ctx context.Context, cfg config) error {
	uri, err := url.Parse(cfg.l1Rpc)
	if err != nil {
		slog.Error("Unable to parse L1 rpc endpoint:", "err", err)
		return err
	}
	slog.Info("Connecting to RPC endpoint", "endpoint", uri.String())

	ethClient, err := ethclient.DialContext(ctx, uri.String())
	if err != nil {
		slog.Error("Unable to connect to L1 rpc endpoint:", "err", err)
		return err
	}

	ipfsClient, err := rpc.NewURLApiWithClient(cfg.ipfsEndpoint, &http.Client{
		Transport: &http.Transport{
			Proxy:             http.ProxyFromEnvironment,
			DisableKeepAlives: true,
		},
	})
	if err != nil {
		slog.Error("Unable to connect to IPFS endpoint:", "err", err)
		return err
	}

	batcherFrom := common.HexToAddress(cfg.batcherAddress)
	batcherInbox := common.HexToAddress(cfg.batcherInbox)

	slog.Info("Using batcher addresses", "batcher", batcherFrom, "inbox", batcherInbox)

	currentBlock := big.NewInt(int64(cfg.startBlock))

	storedBlock, err := getLastProcessedBlock(cfg.lastBlockPath)
	if err != nil {
		slog.Error("Unable to read last block from file, using cli param value", "err", err)
	} else {
		currentBlock = big.NewInt(int64(*storedBlock))
		slog.Info("Using stored last block as the fisrt block to process", "blockNumber", currentBlock)
	}
BLOCKLOOP:
	for {
		reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)

		slog.Debug("Getting batcher transactions", "block", currentBlock)
		block, err := ethClient.BlockByNumber(reqCtx, currentBlock)
		cancel()
		if err != nil {
			slog.Error("Unable to fetch block, retrying in a minute", "block", currentBlock, "err", err)
			time.Sleep(1 * time.Minute)
			continue BLOCKLOOP
		}

		if block == nil {
			slog.Info("Block doesn't exist, skipping", "block", block.Number())
			currentBlock.Add(currentBlock, big.NewInt(1))
			continue
		}

		if block.Time() > uint64(time.Now().Unix()-1800) {
			slog.Info("Reached block 30 minutes from current one, waiting one minute")
			time.Sleep(1 * time.Minute)
			continue
		}

		for _, tx := range block.Transactions() {
			valid, err := isValidBatcherTx(tx, batcherFrom, batcherInbox)
			if err != nil {
				return err
			}

			if !valid {
				continue
			}

			prefix := tx.Data()[0]
			if prefix != 0xfc {
				slog.Warn("Skipping non fraxDA tx", "prefix", fmt.Sprintf("0x%x", prefix))
				continue
			}

			ipfsId, err := bytesToCIDString(tx.Data()[1:])
			if err != nil {
				slog.Error("Can't convert IPFS hash from tx data to string ID", "err", err)
				time.Sleep(5 * time.Minute)
				continue BLOCKLOOP
			}

			_, ipfsCid, err := cid.CidFromBytes(tx.Data()[1:])
			if err != nil {
				slog.Error("Can't convert IPFS CID string to cid object", "err", err, "cid", ipfsId)
				return err
			}

			ipfsPath := ipfspath.FromCid(ipfsCid)
			err = ipfsClient.Pin().Add(ctx, ipfsPath)
			if err != nil {
				slog.Error("Unable to pin IPFS path, retrying in 10 seconds", "err", err, "cid", ipfsId)
				time.Sleep(10 * time.Second)
				continue BLOCKLOOP
			}
			slog.Info("Pinned IPFS data", "txhash", tx.Hash(), "cid", ipfsId)
			if err := storeLastProcessedBlock(cfg.lastBlockPath, currentBlock.Uint64()); err != nil {
				slog.Error("Unable to write current block file", "err", err)
			}
		}

		currentBlock.Add(currentBlock, big.NewInt(1))
	}
}

func isValidBatcherTx(tx *types.Transaction, batcher common.Address, inbox common.Address) (bool, error) {
	to := tx.To()
	if to == nil || *to != inbox {
		return false, nil
	}

	from, err := getFromAddress(tx)
	if err != nil {
		return false, fmt.Errorf("cannot decode from address from tx: %w", err)
	}

	if from != batcher {
		return false, nil
	}

	return true, nil
}

func getFromAddress(tx *types.Transaction) (common.Address, error) {
	from, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	return from, err
}

func bytesToCIDString(b []byte) (string, error) {
	ipfsCID, err := multibase.Encode(multibase.Base32, b)
	if err != nil {
		return "", err
	}
	return ipfsCID, nil
}

func getLastProcessedBlock(filePath string) (*uint64, error) {
	fileContents, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("unable to open last processed block file %s: %w", filePath, err)
	}

	lastBlock, err := strconv.ParseUint(string(fileContents), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("unable to decode last processed block file %s: %w", filePath, err)
	}

	return &lastBlock, nil
}

func storeLastProcessedBlock(filePath string, lastProcessedBlock uint64) error {
	err := os.WriteFile(filePath, []byte(fmt.Sprintf("%d", lastProcessedBlock)), 0644)
	if err != nil {
		return fmt.Errorf("unable to open last processed block file %s: %w", filePath, err)
	}

	return nil
}
