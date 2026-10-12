# Headless Z21 LAN simulator (Go). UDP API on 21105 and 21106; gRPC control on 50051.

FROM golang:1.24-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY api ./api
COPY cmd ./cmd
COPY internal ./internal
COPY simclient ./simclient

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /z21-sim ./cmd/z21-sim && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /z21-sim-ctl ./cmd/z21-sim-ctl

FROM debian:bookworm-slim
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /z21-sim /usr/local/bin/z21-sim
COPY --from=builder /z21-sim-ctl /usr/local/bin/z21-sim-ctl

EXPOSE 21105/udp
EXPOSE 21106/udp
EXPOSE 50051/tcp

ENTRYPOINT ["/usr/local/bin/z21-sim"]
CMD ["-addr", "0.0.0.0:21105", "-alt-addr", "0.0.0.0:21106", "-grpc-addr", "127.0.0.1:50051"]
