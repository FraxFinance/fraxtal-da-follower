# Fraxtal DA follower

This repo contains a tool that allows anyone to support the fraxtal DA by being an active replica and store the IPFS data blobs

### Getting started

#### With docker compose

Clone the repo or just copy the docker-compose.yml file and run `docker-compose up -d`. This starts both an IPFS node and the DA follower

#### Without docker compose

In case you already have a running IPFS node or you don't want to use docker you can download the binary from the [latest release](https://github.com/FraxFinance/fraxtal-da-follower/releases/latest) and run it.

### Usage

Available params:

```
   --ipfs-endpoint value    IPFS kubo endpoint (default: "http://127.0.0.1:5001")
   --l1-rpc value           L1 RPC url (default: "https://ethereum-rpc.publicnode.com")
   --batcher-address value  Batcher address (default: "0x6017f75108f251a488B045A7ce2a7C15b179d1f2")
   --batcher-inbox value    Batcher inbox address (default: "0xfF000000000000000000000000000000000420fC")
   --start-block value      Starting block (L1 block of L2 gneesis) (default: 19135636)
   --debug                  Enabled debug logging (default: false)
   --last-block-path value  A path to a file to store the last processed block (default: "./last-block")
   --help, -h               show help
```
