FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder
ARG TARGETOS TARGETARCH

RUN apk add --update --no-cache build-base

WORKDIR /app

COPY . .

RUN --mount=target=. \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /bin/fraxtal-da-follower .

FROM alpine:3.20

COPY --from=builder /bin/fraxtal-da-follower /bin/fraxtal-da-follower

CMD ["fraxtal-da-follower"]
