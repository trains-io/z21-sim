package sim_test

import (
	"testing"

	"github.com/trains-io/z21-sim/internal/sim"
	"github.com/trains-io/z21.go/protocol"
)

func TestNewState(t *testing.T) {
	st := sim.NewState()
	if !st.TrackPower() {
		t.Fatal("TrackPower() = false, want on after power-up")
	}
	want := protocol.SystemState{
		MainCurrent:         100,
		FilteredMainCurrent: 80,
		Temperature:         42,
		SupplyVoltage:       18000,
		VCCVoltage:          17500,
		Capabilities:        protocol.CapabilityDCC | protocol.CapabilityLocoCmds | protocol.CapabilityAccessoryCmds,
	}
	if got := st.System(); got != want {
		t.Fatalf("System() = %+v, want %+v", got, want)
	}
}

func TestStateSetSystemValues(t *testing.T) {
	st := sim.NewState()
	temperature, current := int16(55), int16(250)

	changes := st.SetSystemValues(sim.SystemValues{Temperature: &temperature, MainCurrent: &current})
	want := sim.NewState().System()
	want.Temperature, want.MainCurrent = 55, 250
	if got := st.System(); got != want {
		t.Fatalf("System() = %+v, want %+v (other values unchanged)", got, want)
	}
	if len(changes) != 1 || changes[0].Flag != protocol.BroadcastFlagSystemState || len(changes[0].Messages) != 1 {
		t.Fatalf("changes = %+v, want one system-state change", changes)
	}
	if state, err := protocol.ParseSystemState(changes[0].Messages[0].Data); err != nil || state != want {
		t.Fatalf("broadcast system state = %+v, err=%v; want %+v", state, err, want)
	}

	if again := st.SetSystemValues(sim.SystemValues{Temperature: &temperature}); again != nil {
		t.Fatalf("SetSystemValues with the same value = %+v, want no changes", again)
	}
	if none := st.SetSystemValues(sim.SystemValues{}); none != nil {
		t.Fatalf("SetSystemValues without values = %+v, want no changes", none)
	}
}

func TestStateTrackPowerOffReportsNoLoad(t *testing.T) {
	st := sim.NewState()
	live := st.System()

	changes := st.SetTrackPower(false)
	off := st.System()
	want := live
	want.CentralState = protocol.CentralStateTrackVoltageOff
	want.MainCurrent, want.ProgCurrent, want.FilteredMainCurrent, want.VCCVoltage = 0, 0, 0, 0
	if off != want {
		t.Fatalf("System() with track power off = %+v, want %+v (no currents, no track voltage; supply and temperature kept)", off, want)
	}
	if state, err := protocol.ParseSystemState(changes[1].Messages[0].Data); err != nil || state != want {
		t.Fatalf("system state broadcast = %+v, err=%v; want %+v", state, err, want)
	}

	current, vcc := int16(1500), uint16(16500)
	if changes := st.SetSystemValues(sim.SystemValues{MainCurrent: &current, VCCVoltage: &vcc}); changes != nil {
		t.Fatalf("SetSystemValues(current, VCC) with track power off = %+v, want no changes (not reported)", changes)
	}
	if got := st.System(); got != want {
		t.Fatalf("System() = %+v, want still %+v", got, want)
	}

	temperature := int16(30)
	if changes := st.SetSystemValues(sim.SystemValues{Temperature: &temperature}); len(changes) != 1 {
		t.Fatalf("SetSystemValues(temperature) with track power off = %+v, want one system-state change", changes)
	}

	st.SetTrackPower(true)
	want = live
	want.MainCurrent, want.VCCVoltage, want.Temperature = 1500, 16500, 30
	if got := st.System(); got != want {
		t.Fatalf("System() after track power on = %+v, want %+v (values set while off)", got, want)
	}
}

func TestStateSetEmergencyStop(t *testing.T) {
	st := sim.NewState()

	changes := st.SetEmergencyStop()
	if !st.EmergencyStopped() || !st.TrackPower() {
		t.Fatalf("EmergencyStopped() = %v, TrackPower() = %v; want stopped with track voltage on", st.EmergencyStopped(), st.TrackPower())
	}
	if len(changes) != 2 {
		t.Fatalf("got %d changes, want X-Bus and system state: %+v", len(changes), changes)
	}
	xbus := changes[0]
	if xbus.Flag != protocol.BroadcastFlagXpressNet || len(xbus.Messages) != 2 || !protocol.IsBCStopped(xbus.Messages[0].Data) {
		t.Fatalf("X-Bus change = %+v, want LAN_X_BC_STOPPED and X status", xbus)
	}
	if status, err := protocol.ParseXStatus(xbus.Messages[1].Data); err != nil || status.CentralState != protocol.CentralStateEmergencyStop {
		t.Fatalf("X status = %+v, err=%v; want emergency stop only", status, err)
	}
	if state, err := protocol.ParseSystemState(changes[1].Messages[0].Data); err != nil || state != st.System() {
		t.Fatalf("system state broadcast = %+v, err=%v; want %+v", state, err, st.System())
	}
	if again := st.SetEmergencyStop(); again != nil {
		t.Fatalf("SetEmergencyStop again = %+v, want no changes", again)
	}

	st.SetTrackPower(false)
	if got := st.System().CentralState; got != protocol.CentralStateEmergencyStop|protocol.CentralStateTrackVoltageOff {
		t.Fatalf("central state after power off = %#x, want emergency stop kept and track voltage off", got)
	}

	if changes := st.SetTrackPower(true); len(changes) != 2 || !st.TrackPower() || st.EmergencyStopped() {
		t.Fatalf("SetTrackPower(true) = %+v, central state %#x; want power on, emergency stop ended", changes, st.System().CentralState)
	}

	st.SetEmergencyStop()
	changes = st.SetTrackPower(true)
	if len(changes) != 2 || st.EmergencyStopped() {
		t.Fatalf("SetTrackPower(true) with power already on = %+v, stopped %v; want emergency stop ended and broadcast", changes, st.EmergencyStopped())
	}
	if on, err := protocol.ParseTrackPowerBC(changes[0].Messages[0].Data); err != nil || !on {
		t.Fatalf("track power broadcast = %v, err=%v; want LAN_X_BC_TRACK_POWER_ON", on, err)
	}
}

func TestStateSetShortCircuit(t *testing.T) {
	st := sim.NewState()
	const shortAndOff = protocol.CentralStateShortCircuit | protocol.CentralStateTrackVoltageOff

	changes := st.SetShortCircuit(sim.ShortCircuitInternal)
	system := st.System()
	if system.CentralState != shortAndOff || system.CentralStateEx != protocol.CentralStateExShortCircuitInternal {
		t.Fatalf("central state = %#x, ex = %#x; want short circuit with track voltage off, internal", system.CentralState, system.CentralStateEx)
	}
	if system.MainCurrent != 0 || system.VCCVoltage != 0 {
		t.Fatalf("system state = %+v, want no current and no track voltage", system)
	}
	if len(changes) != 2 {
		t.Fatalf("got %d changes, want X-Bus and system state: %+v", len(changes), changes)
	}
	xbus := changes[0]
	if xbus.Flag != protocol.BroadcastFlagXpressNet || len(xbus.Messages) != 2 || !protocol.IsBCShortCircuit(xbus.Messages[0].Data) {
		t.Fatalf("X-Bus change = %+v, want LAN_X_BC_TRACK_SHORT_CIRCUIT and X status", xbus)
	}
	if status, err := protocol.ParseXStatus(xbus.Messages[1].Data); err != nil || status.CentralState != shortAndOff {
		t.Fatalf("X status = %+v, err=%v; want %#x", status, err, shortAndOff)
	}
	if state, err := protocol.ParseSystemState(changes[1].Messages[0].Data); err != nil || state != system {
		t.Fatalf("system state broadcast = %+v, err=%v; want %+v", state, err, system)
	}

	if again := st.SetShortCircuit(sim.ShortCircuitInternal); again != nil {
		t.Fatalf("SetShortCircuit(internal) again = %+v, want no changes", again)
	}
	if changes := st.SetShortCircuit(sim.ShortCircuitExternal); len(changes) != 2 {
		t.Fatalf("SetShortCircuit(external) after internal = %+v, want broadcasts", changes)
	}
	if ex := st.System().CentralStateEx; ex != protocol.CentralStateExShortCircuitExternal {
		t.Fatalf("CentralStateEx = %#x, want external only", ex)
	}

	if changes := st.SetTrackPower(true); len(changes) != 2 {
		t.Fatalf("SetTrackPower(true) = %+v, want broadcasts", changes)
	}
	if system := st.System(); system.CentralState != 0 || system.CentralStateEx != 0 || system.MainCurrent == 0 {
		t.Fatalf("system state after track power on = %+v, want short circuit ended and current back", system)
	}
}

func TestStateSetTrackPower(t *testing.T) {
	tests := []struct {
		name         string
		on           bool
		centralState byte
	}{
		{name: "off", on: false, centralState: protocol.CentralStateTrackVoltageOff},
		{name: "on", on: true, centralState: 0},
	}
	st := sim.NewState()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes := st.SetTrackPower(tt.on)
			if st.TrackPower() != tt.on {
				t.Fatalf("TrackPower() = %v, want %v", st.TrackPower(), tt.on)
			}
			if len(changes) != 2 {
				t.Fatalf("got %d changes, want X-Bus and system state: %+v", len(changes), changes)
			}

			xbus := changes[0]
			if xbus.Flag != protocol.BroadcastFlagXpressNet || len(xbus.Messages) != 2 {
				t.Fatalf("X-Bus change = %+v, want track power and X status", xbus)
			}
			if on, err := protocol.ParseTrackPowerBC(xbus.Messages[0].Data); err != nil || on != tt.on {
				t.Fatalf("track power broadcast = %v, err=%v; want %v", on, err, tt.on)
			}
			if status, err := protocol.ParseXStatus(xbus.Messages[1].Data); err != nil || status.CentralState != tt.centralState {
				t.Fatalf("X status = %+v, err=%v; want central state %#x", status, err, tt.centralState)
			}

			sys := changes[1]
			if sys.Flag != protocol.BroadcastFlagSystemState || len(sys.Messages) != 1 {
				t.Fatalf("system state change = %+v", sys)
			}
			state, err := protocol.ParseSystemState(sys.Messages[0].Data)
			if err != nil || state != st.System() {
				t.Fatalf("system state broadcast = %+v, err=%v; want %+v", state, err, st.System())
			}

			if again := st.SetTrackPower(tt.on); again != nil {
				t.Fatalf("SetTrackPower(%v) again = %+v, want no changes", tt.on, again)
			}
		})
	}
}
