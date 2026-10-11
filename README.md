# z21-sim

Go implementation of a [Z21 LAN protocol](https://www.z21.eu) test server, aiming for feature parity with the C++ reference server [z21-ref](https://github.com/trains-io/z21-ref) and room to grow.

The simulator listens on UDP ports **21105** and **21106** (same as a physical Z21; both share client sessions). It reuses wire-format types from [z21.go](https://github.com/trains-io/z21.go).

A **gRPC control plane** on port **50051** lets a sidecar (`z21-sim-controller`) inject unsolicited broadcast events (for example from a Custom Resource in Kubernetes) without sending Z21 UDP directly to gateway clients.

## Prerequisites

- Go 1.24+
- Docker (for integration tests and container usage)

## Quick start

### Local binary

```bash
make run
```

### Docker

```bash
make docker-build
make docker-run
```

Stop the container:

```bash
make docker-stop
```

## Development

```bash
make test              # unit tests
make test-integration  # build image + run integration tests
make build             # compile binary to bin/
make proto             # regenerate gRPC stubs
```

## Drop-in for z21.go integration tests

[z21.go](https://github.com/trains-io/z21.go) integration tests accept a simulator image via `Z21_TESTSERVER_IMAGE`:

```bash
# From z21.go repository
export Z21_TESTSERVER_IMAGE=ghcr.io/trains-io/z21-sim:latest
make test-integration
```

Or build locally from a sibling checkout:

```bash
export Z21_TESTSERVER_IMAGE=z21-sim:local
make -C ../z21-sim docker-build TAG=local
make test-integration
```

Alternatively, point at a Dockerfile context:

```bash
export Z21_TESTSERVER_DOCKERFILE=/path/to/z21-sim
go test -tags=integration ./client/... -count=1 -timeout=10m
```

## Container ports

| Port | Protocol | Purpose |
|------|----------|---------|
| 21105 | UDP | Z21 LAN API |
| 21106 | UDP | Z21 LAN API, alternate port (shares client sessions with 21105) |
| 50051 | TCP | gRPC control plane (localhost sidecar in pod) |

## License

[MIT](LICENSE)
