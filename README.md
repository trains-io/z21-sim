## z21-sim-go

Go implementation of a [Z21 LAN protocol](https://www.z21.eu) test server, intended to replace [z21-sim](https://github.com/trains-io/z21-sim) with feature parity and room to grow.

The simulator listens on UDP port **21105** (same as a physical Z21). It reuses wire-format types from [z21.go](https://github.com/trains-io/z21.go).

A future **gRPC control plane** will let a sidecar (`z21-sim-controller`) inject unsolicited broadcast events (for example from a Custom Resource in Kubernetes) without sending Z21 UDP directly to gateway clients.

### Prerequisites

- Go 1.24+

### Quick start

```bash
make run
```

In another terminal, probe with z21cli or any Z21 LAN client on `127.0.0.1:21105`.

### Development

```bash
make test    # unit tests
make build   # compile binary to bin/
```

### Roadmap

Incremental milestones (each intended as a small commit):

1. **Scaffold** — module layout, UDP listener, `LAN_GET_HWINFO`
2. **Identity** — `LAN_GET_SERIAL_NUMBER` (999-prefix sim serial), `LAN_GET_CODE`
3. **System state** — `LAN_SYSTEMSTATE_GETDATA`, broadcast flags, client registry
4. **XpressNet** — `LAN_X_GET_FIRMWARE_VERSION`, `LAN_X_GET_STATUS`, track power
5. **Broadcasts** — unsolicited `LAN_SYSTEMSTATE_DATACHANGED` to subscribed clients
6. **gRPC control** — proto + service for sidecar-triggered events
7. **Container** — Dockerfile, CI, drop-in for z21.go integration tests

### License

[MIT](LICENSE)
