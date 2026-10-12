# Contributing

Thanks for contributing to [github.com/trains-io/z21-sim](https://github.com/trains-io/z21-sim).

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.24+ | Build and test |
| Docker | recent | Integration tests and container image |
| [grpcurl](https://github.com/fullstorydev/grpcurl) | recent | Manual gRPC control-plane testing |
| [buf](https://buf.build/docs/installation) + `protoc-gen-go`, `protoc-gen-go-grpc` | optional | Regenerate gRPC stubs (`make proto`), lint and breaking-change checks (`make proto-lint`, `make proto-breaking`) |

For end-to-end LAN testing, [z21cli](https://github.com/trains-io/z21cli) is useful but not required.

## Build

From the repository root:

```bash
make build    # bin/z21-sim
```

Equivalent manual command:

```bash
go build -o bin/z21-sim ./cmd/z21-sim
```

## Test

Run the same checks as CI (gofmt, `go vet`, unit tests with the race detector) before pushing:

```bash
make check
```

Or run only the unit tests:

```bash
make test     # go test -race ./... -count=1
```

Run Docker integration tests (builds the image first):

```bash
make test-integration
```

Use `-count=1` while iterating to disable the test cache.

## Run locally

Start the simulator (UDP **21105** and **21106**, gRPC **50051**):

```bash
make run
```

Flags:

| Flag | Default | Purpose |
|------|---------|---------|
| `-addr` | `0.0.0.0:21105` | Z21 LAN UDP listen address |
| `-alt-addr` | `0.0.0.0:21106` | Second Z21 LAN UDP listen address, sharing client sessions (`""` disables) |
| `-grpc-addr` | `127.0.0.1:50051` | gRPC control listen address (`""` disables gRPC) |
| `-trace` | `false` | Dump LAN and gRPC traffic to stderr |

Trace mode is helpful when debugging control-plane injections:

```bash
make run-trace
# or
./bin/z21-sim -addr 127.0.0.1:21105 -alt-addr 127.0.0.1:21106 -grpc-addr 127.0.0.1:50051 -trace
```

With `-trace`, incoming gRPC requests and outgoing LAN broadcasts are printed on stderr (even when no UDP client is subscribed).

## Test the gRPC control plane with grpcurl

The control API is defined in [`api/simctl/v1/simctl.proto`](api/simctl/v1/simctl.proto). The simulator does not enable gRPC server reflection, so grpcurl needs the proto file on the command line.

Install grpcurl (examples):

```bash
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

Set a convenience variable for the repo root:

```bash
REPO_ROOT="$(pwd)"
GRPC_ADDR=127.0.0.1:50051
```

Common grpcurl options used below:

- `-plaintext` — no TLS (local development only)
- `-import-path "$REPO_ROOT"` — proto import root
- `-proto api/simctl/v1/simctl.proto` — service definition

### List services and methods

```bash
grpcurl -plaintext \
  -import-path "$REPO_ROOT" \
  -proto api/simctl/v1/simctl.proto \
  list

grpcurl -plaintext \
  -import-path "$REPO_ROOT" \
  -proto api/simctl/v1/simctl.proto \
  describe trains.io.z21.simctl.v1.SimulatorService
```

### BroadcastSystemState

Emits `LAN_SYSTEMSTATE_DATACHANGED` to clients subscribed to system-state updates.

```bash
grpcurl -plaintext \
  -import-path "$REPO_ROOT" \
  -proto api/simctl/v1/simctl.proto \
  -d '{}' \
  "$GRPC_ADDR" \
  trains.io.z21.simctl.v1.SimulatorService/BroadcastSystemState
```

Example response:

```json
{
  "recipientCount": "1"
}
```

`recipientCount` is the number of UDP clients that received the broadcast (0 if nobody is subscribed).

### SetTrackPower

Turns track power on or off and emits unsolicited XpressNet and system-state broadcasts.

```bash
# Track off
grpcurl -plaintext \
  -import-path "$REPO_ROOT" \
  -proto api/simctl/v1/simctl.proto \
  -d '{"on": false}' \
  "$GRPC_ADDR" \
  trains.io.z21.simctl.v1.SimulatorService/SetTrackPower

# Track on
grpcurl -plaintext \
  -import-path "$REPO_ROOT" \
  -proto api/simctl/v1/simctl.proto \
  -d '{"on": true}' \
  "$GRPC_ADDR" \
  trains.io.z21.simctl.v1.SimulatorService/SetTrackPower
```

Example response:

```json
{
  "on": true,
  "changed": true
}
```

### NotifyCANDeviceDetected

Registers a CAN device and broadcasts its initial status, emulating hot-plug detection.

**Detector** — NetID `0xDB04` (56068), module address 60, two ports:

```bash
grpcurl -plaintext \
  -import-path "$REPO_ROOT" \
  -proto api/simctl/v1/simctl.proto \
  -d '{
    "netId": 56068,
    "kind": "CAN_DEVICE_KIND_DETECTOR",
    "name": "Block 4",
    "moduleAddr": 60,
    "portCount": 2
  }' \
  "$GRPC_ADDR" \
  trains.io.z21.simctl.v1.SimulatorService/NotifyCANDeviceDetected
```

**Booster** — NetID `0xBC01` (48129):

```bash
grpcurl -plaintext \
  -import-path "$REPO_ROOT" \
  -proto api/simctl/v1/simctl.proto \
  -d '{
    "netId": 48129,
    "kind": "CAN_DEVICE_KIND_BOOSTER",
    "name": "Booster 1",
    "outputPort": 1
  }' \
  "$GRPC_ADDR" \
  trains.io.z21.simctl.v1.SimulatorService/NotifyCANDeviceDetected
```

Example response:

```json
{
  "recipientCount": "1"
}
```

grpcurl accepts camelCase JSON field names (`netId`, `moduleAddr`, …) as shown above. Snake_case (`net_id`, `module_addr`, …) also works.

## End-to-end: gRPC injection + LAN client

This workflow verifies that a control-plane RPC produces LAN traffic a Z21 client can see.

**Terminal 1 — simulator with trace:**

```bash
make run-trace
```

**Terminal 2 — Z21 client** ([z21cli](https://github.com/trains-io/z21cli)):

```bash
./bin/z21 context add sim --host 127.0.0.1 --use
./bin/z21 sub add CAN_DETECTOR_UPDATES
./bin/z21 monitor
```

**Terminal 3 — inject a detector via grpcurl:**

```bash
grpcurl -plaintext \
  -import-path "$(pwd)" \
  -proto api/simctl/v1/simctl.proto \
  -d '{"netId":56068,"kind":"CAN_DEVICE_KIND_DETECTOR","name":"Block 4","moduleAddr":60,"portCount":2}' \
  127.0.0.1:50051 \
  trains.io.z21.simctl.v1.SimulatorService/NotifyCANDeviceDetected
```

Expected results:

- **Terminal 1** (`-trace`): gRPC request dump, then `LAN >> broadcast CAN detector -> …` with `LAN_CAN_DETECTOR` payloads.
- **Terminal 2** (`monitor`): lines such as `[CAN] Detector 0xDB04 module 60 port 1: free (LAN_CAN_DETECTOR)`.

For system-state or track-power events, subscribe to `SYSTEM_UPDATES` or `XPRESSNET_UPDATES` instead of `CAN_DETECTOR_UPDATES`.

## Regenerate gRPC stubs

After editing `api/simctl/v1/simctl.proto`:

```bash
make proto-lint proto-breaking proto
```

Requires `buf`, `protoc-gen-go`, and `protoc-gen-go-grpc` on your `PATH` (`buf.gen.yaml` uses the local plugins). `make proto-breaking` compares against the `main` branch; CI runs the same checks.

## CI

GitHub Actions runs `make check` and then `make test-integration` (which builds the Docker image) on every push and pull request. Images are published to `ghcr.io/trains-io/z21-sim` on pushes to `main` and on `v*` tags.

## Drop-in for z21.go integration tests

See [README.md](./README.md#drop-in-for-z21go-integration-tests) for using this image with `Z21_TESTSERVER_IMAGE`.
