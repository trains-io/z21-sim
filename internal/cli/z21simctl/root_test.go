package z21simctl_test

import (
	"bytes"
	"context"
	"encoding/json"
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
