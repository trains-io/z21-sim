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
		SupplyVoltage:       10000,
		VCCVoltage:          1000,
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
