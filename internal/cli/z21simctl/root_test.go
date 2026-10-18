package z21simctl_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/trains-io/z21-sim/internal/cli/z21simctl"
	"github.com/trains-io/z21-sim/internal/sim"
	"github.com/trains-io/z21-sim/internal/sim/grpcserver"
	"github.com/trains-io/z21.go/client"
	"github.com/trains-io/z21.go/protocol"
)

// startSimulator runs a simulator with its control API and returns the UDP
// and control addresses.
func startSimulator(t *testing.T) (udpAddr, controlAddr string) {
	t.Helper()

	simulator, err := sim.New("127.0.0.1:0", nil)
	if err != nil {
		t.Fatalf("sim.New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- simulator.Serve(ctx) }()

	control, err := grpcserver.New(simulator, "127.0.0.1:0", nil)
	if err != nil {
		t.Fatalf("grpcserver.New: %v", err)
	}
	go func() { _ = control.Serve() }()

	t.Cleanup(func() {
		control.Stop()
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Log("simulator did not stop cleanly")
		}
		_ = simulator.Close()
	})
	return simulator.Addr().String(), control.Addr().String()
}

// run executes z21-sim-ctl with args and returns its standard output.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := z21simctl.NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), err
}

func mustRun(t *testing.T, args ...string) string {
	t.Helper()
	out, err := run(t, args...)
	if err != nil {
		t.Fatalf("z21-sim-ctl %s: %v", strings.Join(args, " "), err)
	}
	return out
}

// subscribe dials the simulator as a Z21 LAN client with the given
// broadcast flags.
func subscribe(t *testing.T, udpAddr string, flags uint32) *client.Client {
	t.Helper()
	c, err := client.Dial(udpAddr)
	if err != nil {
		t.Fatalf("client.Dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Send(ctx, protocol.SetBroadcastFlags(flags)); err != nil {
		t.Fatalf("SetBroadcastFlags: %v", err)
	}
	if _, err := c.Call(ctx, protocol.GetBroadcastFlags()); err != nil {
		t.Fatalf("GetBroadcastFlags: %v", err)
	}
	return c
}

func TestPower(t *testing.T) {
	_, addr := startSimulator(t)

	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"power", "on"}, want: "track power on (unchanged)\n"},
		{args: []string{"power", "off"}, want: "track power off\n"},
		{args: []string{"power", "off"}, want: "track power off (unchanged)\n"},
		{args: []string{"-o", "json", "power", "on"}, want: "{\n  \"on\": true,\n  \"changed\": true\n}\n"},
	}
	for _, tt := range tests {
		if got := mustRun(t, append([]string{"--addr", addr}, tt.args...)...); got != tt.want {
			t.Errorf("z21-sim-ctl %s = %q, want %q", strings.Join(tt.args, " "), got, tt.want)
		}
	}
}

func TestStop(t *testing.T) {
	udpAddr, addr := startSimulator(t)
	c := subscribe(t, udpAddr, protocol.BroadcastFlagXpressNet)

	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"stop"}, want: "emergency stop\n"},
		{args: []string{"-o", "json", "stop"}, want: "{\n  \"changed\": false\n}\n"},
		{args: []string{"power", "on"}, want: "track power on\n"},
	}
	for _, tt := range tests {
		if got := mustRun(t, append([]string{"--addr", addr}, tt.args...)...); got != tt.want {
			t.Errorf("z21-sim-ctl %s = %q, want %q", strings.Join(tt.args, " "), got, tt.want)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msgs, err := c.ReadPacket(ctx)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	if err := protocol.BCStoppedFromMessages(msgs); err != nil {
		t.Fatalf("subscriber got %#v: %v", msgs, err)
	}
}

func TestShortCircuit(t *testing.T) {
	_, addr := startSimulator(t)

	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"short-circuit"}, want: "short circuit (internal)\n"},
		{args: []string{"short-circuit"}, want: "short circuit (internal) (unchanged)\n"},
		{args: []string{"-o", "json", "short-circuit", "--external"}, want: "{\n  \"kind\": \"external\",\n  \"changed\": true\n}\n"},
		{args: []string{"power", "on"}, want: "track power on\n"},
	}
	for _, tt := range tests {
		if got := mustRun(t, append([]string{"--addr", addr}, tt.args...)...); got != tt.want {
			t.Errorf("z21-sim-ctl %s = %q, want %q", strings.Join(tt.args, " "), got, tt.want)
		}
	}
}

func TestCANAddDetector(t *testing.T) {
	udpAddr, addr := startSimulator(t)
	c := subscribe(t, udpAddr, protocol.BroadcastFlagCANDetector)

	out := mustRun(t, "--addr", addr, "can", "add", "detector", "0xdb04", "--name", "Block 4", "--module-addr", "0x3c", "--ports", "3")
	if want := "detector 0xdb04 added, sent to 1 client\n"; out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msgs, err := c.ReadPacket(ctx)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	reports, err := protocol.CANDetectorReportsFromMessages(msgs)
	if err != nil {
		t.Fatalf("CANDetectorReportsFromMessages: %v", err)
	}
	if len(reports) != 3 {
		t.Fatalf("got %d reports, want one per port (3): %+v", len(reports), reports)
	}
	for i, r := range reports {
		if r.NetID != 0xdb04 || r.Addr != 60 || int(r.Port) != i {
			t.Fatalf("report %d = %+v, want NetID 0xdb04, module address 60, port %d", i, r, i)
		}
	}
}

func TestCANAddBooster(t *testing.T) {
	udpAddr, addr := startSimulator(t)
	c := subscribe(t, udpAddr, protocol.BroadcastFlagCANBooster)

	out := mustRun(t, "--addr", addr, "-o", "json", "can", "add", "booster", "49409", "--output-port", "2")
	if want := "{\n  \"kind\": \"booster\",\n  \"net_id\": 49409,\n  \"recipients\": 1\n}\n"; out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msgs, err := c.ReadPacket(ctx)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	states, err := protocol.CANBoosterSystemStatesFromMessages(msgs)
	if err != nil || len(states) != 1 || states[0].NetID != 0xc101 || states[0].OutputPort != 2 {
		t.Fatalf("booster states = %+v, err=%v; want NetID 0xc101 on output port 2", states, err)
	}
}

func TestSystemSet(t *testing.T) {
	udpAddr, addr := startSimulator(t)
	c := subscribe(t, udpAddr, protocol.BroadcastFlagSystemState)

	out := mustRun(t, "--addr", addr, "system", "set", "--main-current", "1200", "--temperature", "-5", "--supply-voltage", "18000")
	if want := "system state changed, sent to 1 client\n"; out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msgs, err := c.ReadPacket(ctx)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	state, err := protocol.SystemStateFromMessages(msgs)
	if err != nil {
		t.Fatalf("SystemStateFromMessages: %v", err)
	}
	if state.MainCurrent != 1200 || state.Temperature != -5 || state.SupplyVoltage != 18000 {
		t.Fatalf("system state = %+v, want main current 1200, temperature -5, supply 18000", state)
	}

	out = mustRun(t, "--addr", addr, "-o", "json", "system", "set", "--temperature=-5")
	var res struct {
		Changed    bool `json:"changed"`
		Recipients int  `json:"recipients"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil || res.Changed || res.Recipients != 0 {
		t.Fatalf("repeated set = %q (%v), want unchanged and no recipients", out, err)
	}
}

func TestLocoSet(t *testing.T) {
	udpAddr, addr := startSimulator(t)
	c := subscribe(t, udpAddr, protocol.BroadcastFlagXpressNet)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := c.Call(ctx, protocol.GetLocoInfo(3)); err != nil {
		t.Fatalf("GetLocoInfo: %v", err)
	}

	out := mustRun(t, "--addr", addr, "loco", "set", "0x3", "--steps", "28", "--speed", "10", "--reverse", "-f", "0=on,4=on", "-f", "4=off", "--function", "31=on")
	if want := "loco 3: reverse, speed 10/28, F0 F31 on, sent to 1 client\n"; out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
	msgs, err := c.ReadPacket(ctx)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	info, err := protocol.LocoInfoFromMessages(msgs)
	if err != nil {
		t.Fatalf("subscriber got %#v: %v", msgs, err)
	}
	if step, _ := info.SpeedStep(); step != 10 || info.Forward || !info.Headlight || info.FunctionsF1F4 != 0 || info.FunctionsF29F31 != 0x04 {
		t.Fatalf("LAN_X_LOCO_INFO = %+v, want step 10, reverse, F0 and F31 on", info)
	}

	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"--reverse"}, want: "loco 3: reverse, speed 10/28, F0 F31 on (unchanged)\n"},
		{args: []string{"--steps", "128", "--speed", "0", "--forward", "-f", "0=off,31=off"}, want: "loco 3: forward, speed 0/126, functions off, sent to 1 client\n"},
	}
	for _, tt := range tests {
		args := append([]string{"--addr", addr, "loco", "set", "3"}, tt.args...)
		if got := mustRun(t, args...); got != tt.want {
			t.Errorf("z21-sim-ctl loco set 3 %s = %q, want %q", strings.Join(tt.args, " "), got, tt.want)
		}
	}

	out = mustRun(t, "--addr", addr, "-o", "json", "loco", "set", "3", "--speed", "126", "-f", "1=on")
	var res struct {
		Address       uint16 `json:"address"`
		SpeedSteps    int    `json:"speed_steps"`
		Speed         uint8  `json:"speed"`
		EmergencyStop bool   `json:"emergency_stop"`
		Forward       bool   `json:"forward"`
		Functions     []int  `json:"functions"`
		Changed       bool   `json:"changed"`
		Recipients    int    `json:"recipients"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("json output %q: %v", out, err)
	}
	if res.Address != 3 || res.SpeedSteps != 128 || res.Speed != 126 || res.EmergencyStop || !res.Forward ||
		len(res.Functions) != 1 || res.Functions[0] != 1 || !res.Changed || res.Recipients != 1 {
		t.Fatalf("json output = %+v, want loco 3, 128 speed steps, speed 126, forward, F1 on, changed, 1 recipient", res)
	}
}

func TestSystemBroadcast(t *testing.T) {
	udpAddr, addr := startSimulator(t)
	subscribe(t, udpAddr, protocol.BroadcastFlagSystemState)

	if out, want := mustRun(t, "--addr", addr, "system", "broadcast"), "system state sent to 1 client\n"; out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestClients(t *testing.T) {
	udpAddr, addr := startSimulator(t)

	if out, want := mustRun(t, "--addr", addr, "clients"), "no clients\n"; out != want {
		t.Fatalf("output without clients = %q, want %q", out, want)
	}

	c := subscribe(t, udpAddr, protocol.BroadcastFlagSystemState)
	local := c.LocalAddr().String()

	out := mustRun(t, "--addr", addr, "clients")
	if !strings.HasPrefix(out, "ADDR") || !strings.Contains(out, local) || !strings.Contains(out, "0x00000100") {
		t.Fatalf("output = %q, want a table with %s and broadcast flags 0x00000100", out, local)
	}

	var clients []struct {
		Addr           string `json:"addr"`
		BroadcastFlags string `json:"broadcast_flags"`
	}
	out = mustRun(t, "--addr", addr, "-o", "json", "clients")
	if err := json.Unmarshal([]byte(out), &clients); err != nil {
		t.Fatalf("json output %q: %v", out, err)
	}
	if len(clients) != 1 || clients[0].Addr != local || clients[0].BroadcastFlags != "0x00000100" {
		t.Fatalf("clients = %+v, want one client %s with flags 0x00000100", clients, local)
	}
}

func TestAddrFromEnv(t *testing.T) {
	_, addr := startSimulator(t)
	t.Setenv(z21simctl.EnvAddr, addr)

	if out, want := mustRun(t, "clients"), "no clients\n"; out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestErrors(t *testing.T) {
	_, addr := startSimulator(t)

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "power without state", args: []string{"power"}, wantErr: "accepts 1 arg"},
		{name: "power invalid state", args: []string{"power", "maybe"}, wantErr: "invalid argument"},
		{name: "invalid output", args: []string{"-o", "yaml", "clients"}, wantErr: "invalid --output"},
		{name: "system set without values", args: []string{"system", "set"}, wantErr: "no values given"},
		{name: "system set out of range", args: []string{"system", "set", "--temperature", "40000"}, wantErr: "out of range"},
		{name: "system set negative voltage", args: []string{"system", "set", "--vcc-voltage", "-1"}, wantErr: "invalid argument"},
		{name: "can add detector without NETID", args: []string{"can", "add", "detector"}, wantErr: "accepts 1 arg"},
		{name: "can add detector NETID zero", args: []string{"can", "add", "detector", "0"}, wantErr: "invalid NETID"},
		{name: "can add detector NETID not a number", args: []string{"can", "add", "detector", "block4"}, wantErr: "invalid NETID"},
		{name: "can add detector NETID too large", args: []string{"can", "add", "detector", "0x10000"}, wantErr: "invalid NETID"},
		{name: "can add detector too many ports", args: []string{"can", "add", "detector", "1", "--ports", "300"}, wantErr: "port_count 300 out of range"},
		{name: "can add booster name too long", args: []string{"can", "add", "booster", "1", "--name", "a name that is too long"}, wantErr: "longer than 15 bytes"},
		{name: "loco set without ADDR", args: []string{"loco", "set", "--speed", "1"}, wantErr: "accepts 1 arg"},
		{name: "loco set ADDR zero", args: []string{"loco", "set", "0", "--speed", "1"}, wantErr: "invalid ADDR"},
		{name: "loco set ADDR too large", args: []string{"loco", "set", "10000", "--speed", "1"}, wantErr: "invalid ADDR"},
		{name: "loco set nothing to change", args: []string{"loco", "set", "3"}, wantErr: "nothing to change"},
		{name: "loco set forward and reverse", args: []string{"loco", "set", "3", "--forward", "--reverse"}, wantErr: "none of the others can be"},
		{name: "loco set invalid steps", args: []string{"loco", "set", "3", "--steps", "27"}, wantErr: "invalid --steps 27"},
		{name: "loco set speed above mode", args: []string{"loco", "set", "3", "--steps", "14", "--speed", "15"}, wantErr: "speed step 15 out of range [0, 14]"},
		{name: "loco set function out of range", args: []string{"loco", "set", "3", "-f", "32=on"}, wantErr: "invalid --function \"32=on\""},
		{name: "loco set function without state", args: []string{"loco", "set", "3", "-f", "1"}, wantErr: "invalid --function \"1\""},
		{name: "loco set function invalid state", args: []string{"loco", "set", "3", "-f", "1=toggle"}, wantErr: "invalid --function"},
		{name: "unreachable simulator", args: []string{"--addr", "127.0.0.1:1", "--timeout", "200ms", "power", "on"}, wantErr: "set track power"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := run(t, append([]string{"--addr", addr}, tt.args...)...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestCompletion(t *testing.T) {
	const noFileComp = ":4"

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "power state", args: []string{"power", ""}, want: []string{"on", "off"}},
		{name: "output format", args: []string{"-o", ""}, want: []string{"text", "json"}},
		{name: "output format of subcommand", args: []string{"clients", "--output", ""}, want: []string{"text", "json"}},
		{name: "no args", args: []string{"stop", ""}},
		{name: "NETID", args: []string{"can", "add", "detector", ""}},
		{name: "addr", args: []string{"--addr", ""}},
		{name: "timeout of subcommand", args: []string{"power", "--timeout", ""}},
		{name: "numeric flag", args: []string{"system", "set", "--main-current", ""}},
		{name: "name flag", args: []string{"can", "add", "booster", "1", "--name", ""}},
		{name: "loco ADDR", args: []string{"loco", "set", ""}},
		{name: "speed steps", args: []string{"loco", "set", "3", "--steps", ""}, want: []string{"14", "28", "128"}},
		{name: "function flag", args: []string{"loco", "set", "3", "-f", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := complete(t, tt.args...)
			lines := strings.Split(strings.TrimSpace(out), "\n")
			if n := len(lines); n == 0 || lines[n-1] != noFileComp {
				t.Fatalf("output = %q, want it to end with directive %s (no file completion)", out, noFileComp)
			}
			got := lines[:len(lines)-1]
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("completions = %q, want %q", got, tt.want)
			}
		})
	}
}

// complete returns the standard output of the hidden __complete command that
// the shell completion scripts call.
func complete(t *testing.T, args ...string) string {
	t.Helper()
	cmd := z21simctl.NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"__complete"}, args...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("z21-sim-ctl __complete %s: %v", strings.Join(args, " "), err)
	}
	return out.String()
}
