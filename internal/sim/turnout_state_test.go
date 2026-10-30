package sim_test

import (
	"errors"
	"testing"

	"github.com/trains-io/z21-sim/internal/sim"
	"github.com/trains-io/z21.go/protocol"
)

func TestStateTurnoutDefault(t *testing.T) {
	got := sim.NewState().Turnout(7)
	want := protocol.TurnoutInfo{Address: 7, Position: protocol.TurnoutPositionNotSwitched}
	if got != want {
		t.Fatalf("Turnout(7) = %+v, want %+v", got, want)
	}
}

func TestStateSwitchTurnout(t *testing.T) {
	output1 := protocol.TurnoutSwitch{Activate: true}
	output2 := protocol.TurnoutSwitch{Activate: true, Output2: true}
	tests := []struct {
		name  string
		steps []protocol.TurnoutSwitch
		want  protocol.TurnoutPosition
	}{
		{name: "activate output 1", steps: []protocol.TurnoutSwitch{output1}, want: protocol.TurnoutPositionOutput1},
		{name: "activate output 2", steps: []protocol.TurnoutSwitch{output2}, want: protocol.TurnoutPositionOutput2},
		{name: "deactivate keeps the position", steps: []protocol.TurnoutSwitch{output2, {Output2: true}}, want: protocol.TurnoutPositionOutput2},
		{name: "deactivate other output keeps the position", steps: []protocol.TurnoutSwitch{output2, {}}, want: protocol.TurnoutPositionOutput2},
		{name: "deactivate unswitched turnout", steps: []protocol.TurnoutSwitch{{}}, want: protocol.TurnoutPositionNotSwitched},
		{name: "switch back", steps: []protocol.TurnoutSwitch{output2, output1}, want: protocol.TurnoutPositionOutput1},
		{name: "same output again", steps: []protocol.TurnoutSwitch{output1, output1}, want: protocol.TurnoutPositionOutput1},
		{name: "queued switches at once", steps: []protocol.TurnoutSwitch{{Activate: true, Output2: true, Queue: true}}, want: protocol.TurnoutPositionOutput2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := sim.NewState()
			want := protocol.TurnoutInfo{Address: 7, Position: tt.want}

			var info protocol.TurnoutInfo
			for _, sw := range tt.steps {
				var changes []sim.Change
				var err error
				info, changes, err = st.SwitchTurnout(7, sw)
				if err != nil {
					t.Fatalf("SwitchTurnout(7, %+v): %v", sw, err)
				}
				msg := singleXBusMessage(t, changes)
				if got, err := protocol.ParseTurnoutInfo(msg.Data); err != nil || got != info {
					t.Fatalf("broadcast = %+v, err=%v; want LAN_X_TURNOUT_INFO %+v after every command", got, err, info)
				}
			}
			if info != want {
				t.Fatalf("SwitchTurnout = %+v, want %+v", info, want)
			}
			if got := st.Turnout(7); got != want {
				t.Fatalf("Turnout(7) = %+v, want %+v", got, want)
			}
			if got := st.Turnout(8); got.Position != protocol.TurnoutPositionNotSwitched {
				t.Fatalf("Turnout(8) = %+v, want not switched (other turnouts untouched)", got)
			}
		})
	}
}

func TestStateSwitchTurnoutAddressRange(t *testing.T) {
	st := sim.NewState()
	sw := protocol.TurnoutSwitch{Activate: true}

	for _, addr := range []uint16{0, sim.MaxAccessoryAddress} {
		if info, _, err := st.SwitchTurnout(addr, sw); err != nil || info.Position != protocol.TurnoutPositionOutput1 {
			t.Fatalf("SwitchTurnout(%d) = %+v, %v; want output 1", addr, info, err)
		}
	}

	_, changes, err := st.SwitchTurnout(sim.MaxAccessoryAddress+1, sw)
	if !errors.Is(err, sim.ErrInvalidAccessory) || changes != nil {
		t.Fatalf("SwitchTurnout(%d) = %+v, %v; want ErrInvalidAccessory, no changes", sim.MaxAccessoryAddress+1, changes, err)
	}
	if got := st.Turnout(sim.MaxAccessoryAddress + 1); got.Position != protocol.TurnoutPositionNotSwitched {
		t.Fatalf("Turnout(%d) = %+v, want not switched", sim.MaxAccessoryAddress+1, got)
	}
}

func TestStateExtAccessoryDefault(t *testing.T) {
	got := sim.NewState().ExtAccessory(7)
	want := protocol.ExtAccessoryInfo{Address: 7, Status: protocol.ExtAccessoryStatusUnknown}
	if got != want {
		t.Fatalf("ExtAccessory(7) = %+v, want %+v", got, want)
	}
}

func TestStateSetExtAccessory(t *testing.T) {
	st := sim.NewState()
	want := protocol.ExtAccessoryInfo{Address: 7, Value: 0x12, Status: protocol.ExtAccessoryStatusValid}

	for range 2 {
		info, changes, err := st.SetExtAccessory(7, 0x12)
		if err != nil || info != want {
			t.Fatalf("SetExtAccessory(7, 0x12) = %+v, %v; want %+v", info, err, want)
		}
		msg := singleXBusMessage(t, changes)
		if got, err := protocol.ParseExtAccessoryInfo(msg.Data); err != nil || got != want {
			t.Fatalf("broadcast = %+v, err=%v; want LAN_X_EXT_ACCESSORY_INFO %+v after every command", got, err, want)
		}
	}
	if got := st.ExtAccessory(7); got != want {
		t.Fatalf("ExtAccessory(7) = %+v, want %+v", got, want)
	}

	zero := protocol.ExtAccessoryInfo{Address: 7, Status: protocol.ExtAccessoryStatusValid}
	if info, _, err := st.SetExtAccessory(7, 0); err != nil || info != zero {
		t.Fatalf("SetExtAccessory(7, 0) = %+v, %v; want %+v (value 0 is valid)", info, err, zero)
	}
	if got := st.ExtAccessory(8); got.Status != protocol.ExtAccessoryStatusUnknown {
		t.Fatalf("ExtAccessory(8) = %+v, want unknown (other accessories untouched)", got)
	}
}

func TestStateSetExtAccessoryAddressRange(t *testing.T) {
	st := sim.NewState()

	if _, _, err := st.SetExtAccessory(sim.MaxAccessoryAddress, 1); err != nil {
		t.Fatalf("SetExtAccessory(%d): %v", sim.MaxAccessoryAddress, err)
	}
	_, changes, err := st.SetExtAccessory(sim.MaxAccessoryAddress+1, 1)
	if !errors.Is(err, sim.ErrInvalidAccessory) || changes != nil {
		t.Fatalf("SetExtAccessory(%d) = %+v, %v; want ErrInvalidAccessory, no changes", sim.MaxAccessoryAddress+1, changes, err)
	}
	if got := st.ExtAccessory(sim.MaxAccessoryAddress + 1); got.Status != protocol.ExtAccessoryStatusUnknown {
		t.Fatalf("ExtAccessory(%d) = %+v, want unknown", sim.MaxAccessoryAddress+1, got)
	}
}

// singleXBusMessage returns the only message of changes, which must be a
// single X-Bus broadcast.
func singleXBusMessage(t *testing.T, changes []sim.Change) protocol.Message {
	t.Helper()
	if len(changes) != 1 || changes[0].Flag != protocol.BroadcastFlagXpressNet || len(changes[0].Messages) != 1 {
		t.Fatalf("changes = %+v, want one X-Bus broadcast with one message", changes)
	}
	return changes[0].Messages[0]
}
