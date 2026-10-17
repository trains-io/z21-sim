package simclient_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/trains-io/z21-sim/internal/sim"
	"github.com/trains-io/z21-sim/internal/sim/grpcserver"
	"github.com/trains-io/z21-sim/simclient"
	"github.com/trains-io/z21.go/client"
	"github.com/trains-io/z21.go/protocol"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fixture is a running simulator with its control API and a simclient
// connected to it.
type fixture struct {
	sim     *simclient.Client
	udpAddr string
}

func newFixture(t *testing.T) fixture {
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

	c, err := simclient.Dial(control.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	t.Cleanup(func() {
		_ = c.Close()
		control.Stop()
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Log("simulator did not stop cleanly")
		}
		_ = simulator.Close()
	})
	return fixture{sim: c, udpAddr: simulator.Addr().String()}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// lanClient dials the simulator as a Z21 LAN client subscribed to flags (none
// if 0).
func (f fixture) lanClient(t *testing.T, flags uint32) *client.Client {
	t.Helper()
	c, err := client.Dial(f.udpAddr)
	if err != nil {
		t.Fatalf("client.Dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	ctx := testContext(t)
	if err := c.Send(ctx, protocol.SetBroadcastFlags(flags)); err != nil {
		t.Fatalf("SetBroadcastFlags: %v", err)
	}
	if _, err := c.Call(ctx, protocol.GetBroadcastFlags()); err != nil {
		t.Fatalf("GetBroadcastFlags: %v", err)
	}
	return c
}

// systemState asks the simulator for its system state over UDP.
func (f fixture) systemState(t *testing.T) protocol.SystemState {
	t.Helper()
	c := f.lanClient(t, 0)
	msgs, err := c.Call(testContext(t), protocol.SystemStateGetData())
	if err != nil {
		t.Fatalf("SystemStateGetData: %v", err)
	}
	state, err := protocol.SystemStateFromMessages(msgs)
	if err != nil {
		t.Fatalf("SystemStateFromMessages: %v", err)
	}
	return state
}

func TestDialIsLazy(t *testing.T) {
	c, err := simclient.Dial("127.0.0.1:1")
	if err != nil {
		t.Fatalf("Dial without a server = %v, want no error until the first call", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err = c.SetTrackPower(ctx, true)
	if err == nil || !strings.HasPrefix(err.Error(), "simclient: set track power: ") {
		t.Fatalf("SetTrackPower error = %v, want it prefixed with the operation", err)
	}
	if code := status.Code(err); code != codes.Unavailable && code != codes.DeadlineExceeded {
		t.Fatalf("status code = %v, want Unavailable or DeadlineExceeded (wrapped gRPC status kept)", code)
	}
}

func TestSetTrackPower(t *testing.T) {
	f := newFixture(t)
	ctx := testContext(t)

	tests := []struct {
		on          bool
		wantChanged bool
		wantState   byte
	}{
		{on: true, wantChanged: false, wantState: 0},
		{on: false, wantChanged: true, wantState: protocol.CentralStateTrackVoltageOff},
		{on: false, wantChanged: false, wantState: protocol.CentralStateTrackVoltageOff},
		{on: true, wantChanged: true, wantState: 0},
	}
	for _, tt := range tests {
		changed, err := f.sim.SetTrackPower(ctx, tt.on)
		if err != nil {
			t.Fatalf("SetTrackPower(%v): %v", tt.on, err)
		}
		if changed != tt.wantChanged {
			t.Errorf("SetTrackPower(%v) changed = %v, want %v", tt.on, changed, tt.wantChanged)
		}
		if got := f.systemState(t).CentralState; got != tt.wantState {
			t.Errorf("after SetTrackPower(%v): central state = %#x, want %#x", tt.on, got, tt.wantState)
		}
	}
}

func TestEmergencyStop(t *testing.T) {
	f := newFixture(t)
	ctx := testContext(t)

	for i, wantChanged := range []bool{true, false} {
		changed, err := f.sim.EmergencyStop(ctx)
		if err != nil {
			t.Fatalf("EmergencyStop #%d: %v", i+1, err)
		}
		if changed != wantChanged {
			t.Errorf("EmergencyStop #%d changed = %v, want %v", i+1, changed, wantChanged)
		}
	}
	if got := f.systemState(t).CentralState; got != protocol.CentralStateEmergencyStop {
		t.Fatalf("central state = %#x, want emergency stop only", got)
	}

	if changed, err := f.sim.SetTrackPower(ctx, true); err != nil || !changed {
		t.Fatalf("SetTrackPower(true) = %v, %v; want changed (ends the emergency stop)", changed, err)
	}
	if got := f.systemState(t).CentralState; got != 0 {
		t.Fatalf("central state after power on = %#x, want 0", got)
	}
}

func TestShortCircuit(t *testing.T) {
	f := newFixture(t)
	ctx := testContext(t)

	tests := []struct {
		name        string
		kind        simclient.ShortCircuitKind
		wantChanged bool
		wantEx      byte
	}{
		{name: "internal", kind: simclient.ShortCircuitInternal, wantChanged: true, wantEx: protocol.CentralStateExShortCircuitInternal},
		{name: "internal again", kind: simclient.ShortCircuitInternal, wantChanged: false, wantEx: protocol.CentralStateExShortCircuitInternal},
		{name: "external", kind: simclient.ShortCircuitExternal, wantChanged: true, wantEx: protocol.CentralStateExShortCircuitExternal},
	}
	for _, tt := range tests {
		changed, err := f.sim.ShortCircuit(ctx, tt.kind)
		if err != nil {
			t.Fatalf("ShortCircuit(%s): %v", tt.name, err)
		}
		if changed != tt.wantChanged {
			t.Errorf("ShortCircuit(%s) changed = %v, want %v", tt.name, changed, tt.wantChanged)
		}
		state := f.systemState(t)
		if state.CentralState != protocol.CentralStateShortCircuit|protocol.CentralStateTrackVoltageOff || state.CentralStateEx != tt.wantEx {
			t.Errorf("after ShortCircuit(%s): central state = %#x, ex = %#x; want short circuit, track off, ex %#x", tt.name, state.CentralState, state.CentralStateEx, tt.wantEx)
		}
	}
}

func TestSetSystemState(t *testing.T) {
	f := newFixture(t)
	ctx := testContext(t)
	before := f.systemState(t)

	temperature, main, supply := int16(-5), int16(1200), uint16(18000)
	changed, recipients, err := f.sim.SetSystemState(ctx, simclient.SystemValues{
		Temperature:   &temperature,
		MainCurrent:   &main,
		SupplyVoltage: &supply,
	})
	if err != nil {
		t.Fatalf("SetSystemState: %v", err)
	}
	if !changed || recipients != 0 {
		t.Fatalf("SetSystemState = changed %v, %d recipients; want changed, nobody subscribed", changed, recipients)
	}

	want := before
	want.Temperature, want.MainCurrent, want.SupplyVoltage = -5, 1200, 18000
	if got := f.systemState(t); got != want {
		t.Fatalf("system state = %+v, want %+v (only the given values changed)", got, want)
	}

	f.lanClient(t, protocol.BroadcastFlagSystemState)
	vcc := uint16(16500)
	if changed, recipients, err := f.sim.SetSystemState(ctx, simclient.SystemValues{VCCVoltage: &vcc}); err != nil || !changed || recipients != 1 {
		t.Fatalf("SetSystemState(VCC) = %v, %d, %v; want changed, 1 recipient", changed, recipients, err)
	}
	if changed, recipients, err := f.sim.SetSystemState(ctx, simclient.SystemValues{}); err != nil || changed || recipients != 0 {
		t.Fatalf("SetSystemState(no values) = %v, %d, %v; want unchanged, no recipients", changed, recipients, err)
	}
}

func TestBroadcastSystemState(t *testing.T) {
	f := newFixture(t)
	ctx := testContext(t)

	if recipients, err := f.sim.BroadcastSystemState(ctx); err != nil || recipients != 0 {
		t.Fatalf("BroadcastSystemState without subscribers = %d, %v; want 0", recipients, err)
	}

	sub := f.lanClient(t, protocol.BroadcastFlagSystemState)
	f.lanClient(t, protocol.BroadcastFlagXpressNet)
	if recipients, err := f.sim.BroadcastSystemState(ctx); err != nil || recipients != 1 {
		t.Fatalf("BroadcastSystemState = %d, %v; want 1 (the system-state subscriber only)", recipients, err)
	}
	msgs, err := sub.ReadPacket(ctx)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	if _, err := protocol.SystemStateFromMessages(msgs); err != nil {
		t.Fatalf("subscriber got %#v: %v", msgs, err)
	}
}

func TestAddCANDetector(t *testing.T) {
	f := newFixture(t)
	ctx := testContext(t)
	sub := f.lanClient(t, protocol.BroadcastFlagCANDetector)

	recipients, err := f.sim.AddCANDetector(ctx, simclient.CANDetector{NetID: 0xDB04, Name: "Block 4", ModuleAddr: 60, PortCount: 2})
	if err != nil || recipients != 1 {
		t.Fatalf("AddCANDetector = %d, %v; want 1 recipient", recipients, err)
	}

	msgs, err := sub.ReadPacket(ctx)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	reports, err := protocol.CANDetectorReportsFromMessages(msgs)
	if err != nil || len(reports) != 2 || reports[0].NetID != 0xDB04 || reports[0].Addr != 60 {
		t.Fatalf("reports = %+v, err=%v; want 2 ports of NetID 0xdb04, module 60", reports, err)
	}
	if name := canDeviceName(t, f, 0xDB04); name != "Block 4" {
		t.Fatalf("name = %q, want Block 4", name)
	}
}

func TestAddCANBooster(t *testing.T) {
	f := newFixture(t)
	ctx := testContext(t)
	sub := f.lanClient(t, protocol.BroadcastFlagCANBooster)

	recipients, err := f.sim.AddCANBooster(ctx, simclient.CANBooster{NetID: 0xC101, Name: "Booster 1", OutputPort: 2})
	if err != nil || recipients != 1 {
		t.Fatalf("AddCANBooster = %d, %v; want 1 recipient", recipients, err)
	}

	msgs, err := sub.ReadPacket(ctx)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	states, err := protocol.CANBoosterSystemStatesFromMessages(msgs)
	if err != nil || len(states) != 1 || states[0].NetID != 0xC101 || states[0].OutputPort != 2 {
		t.Fatalf("booster states = %+v, err=%v; want NetID 0xc101 on output port 2", states, err)
	}
	if name := canDeviceName(t, f, 0xC101); name != "Booster 1" {
		t.Fatalf("name = %q, want Booster 1", name)
	}
}

func TestAddCANDeviceInvalid(t *testing.T) {
	f := newFixture(t)
	ctx := testContext(t)

	tests := []struct {
		name    string
		add     func() error
		wantErr string
	}{
		{
			name:    "detector NetID zero",
			add:     func() error { _, err := f.sim.AddCANDetector(ctx, simclient.CANDetector{}); return err },
			wantErr: "simclient: add CAN detector: ",
		},
		{
			name: "detector too many ports",
			add: func() error {
				_, err := f.sim.AddCANDetector(ctx, simclient.CANDetector{NetID: 1, PortCount: 300})
				return err
			},
			wantErr: "port_count 300 out of range",
		},
		{
			name: "booster name too long",
			add: func() error {
				_, err := f.sim.AddCANBooster(ctx, simclient.CANBooster{NetID: 1, Name: "sixteen chars!!!"})
				return err
			},
			wantErr: "simclient: add CAN booster: ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.add()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("status code = %v, want InvalidArgument", status.Code(err))
			}
		})
	}
}

func canDeviceName(t *testing.T, f fixture, netID uint16) string {
	t.Helper()
	c := f.lanClient(t, 0)
	msgs, err := c.Call(testContext(t), protocol.GetCANDeviceDescription(netID))
	if err != nil {
		t.Fatalf("GetCANDeviceDescription: %v", err)
	}
	for _, msg := range msgs {
		if msg.Header != protocol.HeaderLANCANDeviceGetDescription {
			continue
		}
		id, name, err := protocol.ParseCANDeviceDescription(msg.Data)
		if err != nil || id != netID {
			t.Fatalf("description = %#x %q, err=%v; want NetID %#x", id, name, err, netID)
		}
		return name
	}
	t.Fatalf("no LAN_CAN_DEVICE_GET_DESCRIPTION reply in %#v", msgs)
	return ""
}

func TestClients(t *testing.T) {
	f := newFixture(t)
	ctx := testContext(t)

	clients, err := f.sim.Clients(ctx)
	if err != nil || len(clients) != 0 {
		t.Fatalf("Clients without sessions = %+v, %v; want none", clients, err)
	}

	c := f.lanClient(t, protocol.BroadcastFlagXpressNet)
	before := time.Now()
	if err := c.Send(ctx, protocol.GetLocoInfo(3)); err != nil {
		t.Fatalf("GetLocoInfo: %v", err)
	}
	if _, err := c.Call(ctx, protocol.GetBroadcastFlags()); err != nil {
		t.Fatalf("GetBroadcastFlags: %v", err)
	}

	clients, err = f.sim.Clients(ctx)
	if err != nil || len(clients) != 1 {
		t.Fatalf("Clients = %+v, %v; want one", clients, err)
	}
	got := clients[0]
	if got.Addr.String() != c.LocalAddr().String() || got.LocalAddr.String() != f.udpAddr {
		t.Errorf("Addr = %v, LocalAddr = %v; want %v, %v", got.Addr, got.LocalAddr, c.LocalAddr(), f.udpAddr)
	}
	if got.BroadcastFlags != protocol.BroadcastFlagXpressNet {
		t.Errorf("BroadcastFlags = %#08x, want X-Bus", got.BroadcastFlags)
	}
	if len(got.Locos) != 1 || got.Locos[0] != 3 {
		t.Errorf("Locos = %v, want [3]", got.Locos)
	}
	if got.LastSeen.Before(before.Add(-time.Second)) || got.LastSeen.After(time.Now()) {
		t.Errorf("LastSeen = %v, want between %v and now", got.LastSeen, before)
	}
}
