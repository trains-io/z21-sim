# Contributing

Thanks for contributing to [github.com/trains-io/z21-sim](https://github.com/trains-io/z21-sim).

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.24+ | Build and test |
| Docker | recent | Integration tests and container image |
| [grpcurl](https://github.com/fullstorydev/grpcurl) | optional | Calling the gRPC control API directly, bypassing `z21-sim-ctl` |
| [buf](https://buf.build/docs/installation) + `protoc-gen-go`, `protoc-gen-go-grpc` | optional | Regenerate gRPC stubs (`make proto`), lint and breaking-change checks (`make proto-lint`, `make proto-breaking`) |

For end-to-end LAN testing, [z21cli](https://github.com/trains-io/z21cli) is useful but not required.

## Build

From the repository root:

```bash
make build    # bin/z21-sim and bin/z21-sim-ctl
```

Equivalent manual commands:

```bash
go build -o bin/z21-sim ./cmd/z21-sim
go build -o bin/z21-sim-ctl ./cmd/z21-sim-ctl
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

Identity flags set what the simulator reports about itself; they are fixed for the life of the process:

| Flag | Default | Purpose |
|------|---------|---------|
| `-serial` | `999` + six digits from the start time | Serial number (`LAN_GET_SERIAL_NUMBER`); the `999` prefix tells simulators from real hardware |
| `-hw-type` | `z21` | Hardware type (`LAN_GET_HWINFO`): `z21`, `z21-old`, `z21-small`, `z21-start`, `z21-xl`, `smartrail`, or a number such as `0x0211` |
| `-firmware` | `1.20` | Firmware version `major.minor` (`LAN_GET_HWINFO` and `LAN_X_GET_FIRMWARE_VERSION`) |
| `-code` | `none` | Lock code (`LAN_GET_CODE`): `none`, `start-locked`, `start-unlocked` |
| `-capabilities` | `0x31` | Capabilities byte in `LAN_SYSTEMSTATE_DATACHANGED` (spec §2.18), decimal or `0x` hex |

For example, a locked z21 start:

```bash
./bin/z21-sim -hw-type z21-start -firmware 1.43 -code start-locked -serial 123456
```

Trace mode is helpful when debugging control-plane injections:

```bash
make run-trace
# or
./bin/z21-sim -addr 127.0.0.1:21105 -alt-addr 127.0.0.1:21106 -grpc-addr 127.0.0.1:50051 -trace
```

With `-trace`, incoming gRPC requests and outgoing LAN broadcasts are printed on stderr (even when no UDP client is subscribed).

## Control the simulator with z21-sim-ctl

`z21-sim-ctl` changes the state of a running simulator through its gRPC control API (defined in [`api/simctl/v1/simctl.proto`](api/simctl/v1/simctl.proto)). Z21 LAN clients subscribed to the matching broadcasts see the changes, as they would with real hardware.

Global flags:

| Flag | Default | Purpose |
|------|---------|---------|
| `--addr` | `127.0.0.1:50051` | Control API address of the simulator (env `Z21_SIM_CTL_ADDR`) |
| `-o`, `--output` | `text` | Output format: `text` or `json` |
| `--timeout` | `5s` | Timeout per request |

Shell completion (bash needs the `bash-completion` package):

```bash
source <(./bin/z21-sim-ctl completion bash)   # or zsh, fish, powershell
```

Run `./bin/z21-sim-ctl <command> --help` for the broadcasts each command sends. Commands that broadcast report how many clients received the message (0 if nobody is subscribed).

### Track power, emergency stop and short circuit

```bash
./bin/z21-sim-ctl power off
./bin/z21-sim-ctl power on
./bin/z21-sim-ctl stop                      # emergency stop, track voltage stays on
./bin/z21-sim-ctl short-circuit             # on the Z21's own track output
./bin/z21-sim-ctl short-circuit --external  # on a booster output
```

`power on` ends an emergency stop and a short circuit. Subscribe to `XPRESSNET_UPDATES` for `LAN_X_BC_*` messages, to `SYSTEM_UPDATES` for `LAN_SYSTEMSTATE_DATACHANGED`.

### System state

Change the currents (mA), voltages (mV) and temperature (°C) reported in `LAN_SYSTEMSTATE_DATACHANGED`; values not given stay unchanged:

```bash
./bin/z21-sim-ctl system set --main-current 1200 --temperature 55
./bin/z21-sim-ctl system set --supply-voltage 18000 --vcc-voltage 16500
./bin/z21-sim-ctl system broadcast          # resend the current state
```

While track power is off, the currents and the VCC (track) voltage are reported as 0.

### CAN devices

Plug in a CAN occupancy detector (all ports free) or a booster, emulating hot-plug detection. NetIDs and numeric flags accept decimal or `0x` hex; a device with the same NetID is replaced.

```bash
./bin/z21-sim-ctl can add detector 0xdb04 --name "Block 4" --module-addr 60 --ports 2
./bin/z21-sim-ctl can add booster 0xbc01 --name "Booster 1" --output-port 1
```

Subscribe to `CAN_DETECTOR_UPDATES` or `CAN_BOOSTER_UPDATES` respectively.

### Locos

Drive a loco as a throttle outside the Z21 LAN would; values not given stay unchanged. `--speed` is a speed step (0 stops, at most 14, 28 or 126); a loco nobody has driven has 128 speed steps. Addresses accept decimal or `0x` hex.

```bash
./bin/z21-sim-ctl loco set 3 --speed 40 --forward
./bin/z21-sim-ctl loco set 3 -f 0=on -f 1=off       # functions F0–F31
./bin/z21-sim-ctl loco set 1234 --steps 28 --speed 10 --reverse
```

`LAN_X_LOCO_INFO` goes to clients that asked for the loco with `LAN_X_GET_LOCO_INFO` and subscribe to `XPRESSNET_UPDATES`, and to clients subscribed to `ALL_LOCO_INFO_UPDATES`.

### Clients

List the LAN clients with an active session, their broadcast flags and subscribed locos:

```bash
./bin/z21-sim-ctl clients
./bin/z21-sim-ctl -o json clients
```

### In the container

The image includes `z21-sim-ctl`; the control API listens on 127.0.0.1 inside the container:

```bash
docker exec <container> z21-sim-ctl power off
```

### Calling the API directly with grpcurl

To debug the control API itself, call it with [grpcurl](https://github.com/fullstorydev/grpcurl). The simulator does not enable gRPC server reflection, so grpcurl needs the proto file:

```bash
grpcurl -plaintext -import-path . -proto api/simctl/v1/simctl.proto \
  describe trains.io.z21.simctl.v1.SimulatorService

grpcurl -plaintext -import-path . -proto api/simctl/v1/simctl.proto \
  -d '{"on": false}' \
  127.0.0.1:50051 trains.io.z21.simctl.v1.SimulatorService/SetTrackPower
```

## End-to-end: z21-sim-ctl + LAN client

This workflow verifies that a control-plane command produces LAN traffic a Z21 client can see.

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

**Terminal 3 — plug in a detector:**

```bash
./bin/z21-sim-ctl can add detector 0xdb04 --name "Block 4" --module-addr 60 --ports 2
# detector 0xdb04 added, sent to 1 clients
```

Expected results:

- **Terminal 1** (`-trace`): gRPC request dump, then `LAN >> broadcast CAN detector -> …` with `LAN_CAN_DETECTOR` payloads.
- **Terminal 2** (`monitor`): lines such as `[CAN] Detector 0xDB04 module 60 port 1: free (LAN_CAN_DETECTOR)`.

`z21 monitor` does not send keep-alives, so the simulator drops its session after 60 s without other traffic; run it again if broadcasts stop arriving.

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
