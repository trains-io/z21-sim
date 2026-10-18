package sim_test

import (
	"testing"

	"github.com/trains-io/z21-sim/internal/sim"
	"github.com/trains-io/z21.go/protocol"
)

func TestStateLocoDefault(t *testing.T) {
	got := sim.NewState().Loco(3)
	want := protocol.LocoInfo{Address: 3, SpeedSteps: 0x04, Forward: true}
	if got != want {
		t.Fatalf("Loco(3) = %+v, want %+v (128 speed steps, forward, stopped)", got, want)
	}
}

func TestStateDriveLoco(t *testing.T) {
	tests := []struct {
		name      string
		steps     byte
		wantSteps byte
	}{
		{name: "14 speed steps", steps: protocol.LocoSpeedSteps14, wantSteps: 0x00},
		{name: "28 speed steps", steps: protocol.LocoSpeedSteps28, wantSteps: 0x02},
		{name: "128 speed steps", steps: protocol.LocoSpeedSteps128, wantSteps: 0x04},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := sim.NewState()
			drive := protocol.LocoDrive{Address: 3, SpeedSteps: tt.steps, Speed: 20}

			info, changed := st.DriveLoco(drive)
			want := protocol.LocoInfo{Address: 3, SpeedSteps: tt.wantSteps, Speed: 20}
			if info != want || !changed {
				t.Fatalf("DriveLoco = %+v, %v; want %+v, changed", info, changed, want)
			}
			if got := st.Loco(3); got != want {
				t.Fatalf("Loco(3) = %+v, want %+v", got, want)
			}
			if _, changed := st.DriveLoco(drive); changed {
				t.Fatal("DriveLoco with the same values reported a change")
			}
			if got := st.Loco(4); got != sim.NewState().Loco(4) {
				t.Fatalf("Loco(4) = %+v, want the default (other locos untouched)", got)
			}
		})
	}
}

func TestStateSetLocoFunction(t *testing.T) {
	tests := []struct {
		name string
		fn   uint8
		want protocol.LocoInfo
	}{
		{name: "F0", fn: 0, want: protocol.LocoInfo{Headlight: true}},
		{name: "F1", fn: 1, want: protocol.LocoInfo{FunctionsF1F4: 0x01}},
		{name: "F4", fn: 4, want: protocol.LocoInfo{FunctionsF1F4: 0x08}},
		{name: "F5", fn: 5, want: protocol.LocoInfo{FunctionsF5F12: 0x01}},
		{name: "F12", fn: 12, want: protocol.LocoInfo{FunctionsF5F12: 0x80}},
		{name: "F13", fn: 13, want: protocol.LocoInfo{FunctionsF13F20: 0x01}},
		{name: "F28", fn: 28, want: protocol.LocoInfo{FunctionsF21F28: 0x80}},
		{name: "F31", fn: 31, want: protocol.LocoInfo{FunctionsF29F31: 0x04}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := sim.NewState()
			want := withFunctions(st.Loco(3), tt.want)

			if got, changed := st.SetLocoFunction(3, tt.fn, protocol.LocoFunctionOn); got != want || !changed {
				t.Fatalf("on = %+v, %v; want %+v, changed", got, changed, want)
			}
			if _, changed := st.SetLocoFunction(3, tt.fn, protocol.LocoFunctionOn); changed {
				t.Fatal("on again reported a change")
			}
			if got, _ := st.SetLocoFunction(3, tt.fn, protocol.LocoFunctionToggle); got != sim.NewState().Loco(3) {
				t.Fatalf("toggle = %+v, want all functions off", got)
			}
			st.SetLocoFunction(3, tt.fn, protocol.LocoFunctionToggle)
			if got, _ := st.SetLocoFunction(3, tt.fn, protocol.LocoFunctionOff); got != sim.NewState().Loco(3) {
				t.Fatalf("off = %+v, want all functions off", got)
			}
		})
	}

	t.Run("F32 ignored", func(t *testing.T) {
		if _, changed := sim.NewState().SetLocoFunction(3, 32, protocol.LocoFunctionOn); changed {
			t.Fatal("F32 reported a change; LAN_X_LOCO_INFO has no room for it")
		}
	})
}

func TestStateSetLocoFunctionGroup(t *testing.T) {
	tests := []struct {
		name      string
		group     protocol.LocoFunctionGroup
		functions byte
		want      protocol.LocoInfo
	}{
		{name: "F0-F4", group: protocol.LocoFunctionGroupF0F4, functions: 0x1F, want: protocol.LocoInfo{Headlight: true, FunctionsF1F4: 0x0F}},
		{name: "F0 only", group: protocol.LocoFunctionGroupF0F4, functions: 0x10, want: protocol.LocoInfo{Headlight: true}},
		{name: "F5-F8", group: protocol.LocoFunctionGroupF5F8, functions: 0x0F, want: protocol.LocoInfo{FunctionsF5F12: 0x0F}},
		{name: "F9-F12", group: protocol.LocoFunctionGroupF9F12, functions: 0x0F, want: protocol.LocoInfo{FunctionsF5F12: 0xF0}},
		{name: "F13-F20", group: protocol.LocoFunctionGroupF13F20, functions: 0xFF, want: protocol.LocoInfo{FunctionsF13F20: 0xFF}},
		{name: "F21-F28", group: protocol.LocoFunctionGroupF21F28, functions: 0x81, want: protocol.LocoInfo{FunctionsF21F28: 0x81}},
		{name: "F29-F36 keeps F29-F31", group: protocol.LocoFunctionGroupF29F36, functions: 0xFF, want: protocol.LocoInfo{FunctionsF29F31: 0x07}},
		{name: "F37-F44 ignored", group: protocol.LocoFunctionGroupF37F44, functions: 0xFF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := sim.NewState()
			want := withFunctions(st.Loco(3), tt.want)
			got, changed := st.SetLocoFunctionGroup(3, tt.group, tt.functions)
			if got != want || changed != (want != sim.NewState().Loco(3)) {
				t.Fatalf("SetLocoFunctionGroup = %+v, %v; want %+v", got, changed, want)
			}
		})
	}

	t.Run("other functions kept", func(t *testing.T) {
		st := sim.NewState()
		st.SetLocoFunction(3, 1, protocol.LocoFunctionOn)
		st.SetLocoFunction(3, 5, protocol.LocoFunctionOn)
		got, _ := st.SetLocoFunctionGroup(3, protocol.LocoFunctionGroupF0F4, 0x10)
		want := withFunctions(st.Loco(4), protocol.LocoInfo{Headlight: true, FunctionsF5F12: 0x01})
		want.Address = 3
		if got != want {
			t.Fatalf("SetLocoFunctionGroup(F0-F4) = %+v, want %+v (F1 off, F5 kept)", got, want)
		}
	})
}

func TestStateEStopLoco(t *testing.T) {
	st := sim.NewState()
	st.DriveLoco(protocol.LocoDrive{Address: 3, SpeedSteps: protocol.LocoSpeedSteps28, Speed: 20})

	got, changed := st.EStopLoco(3)
	want := protocol.LocoInfo{Address: 3, SpeedSteps: 0x02, Speed: 0x01}
	if got != want || !changed {
		t.Fatalf("EStopLoco = %+v, %v; want %+v (direction kept, e-stop speed), changed", got, changed, want)
	}
	if _, changed := st.EStopLoco(3); changed {
		t.Fatal("EStopLoco again reported a change")
	}
}

func TestStatePurgeLoco(t *testing.T) {
	st := sim.NewState()
	st.DriveLoco(protocol.LocoDrive{Address: 3, SpeedSteps: protocol.LocoSpeedSteps28, Speed: 20})
	st.SetLocoFunction(3, 0, protocol.LocoFunctionOn)

	st.PurgeLoco(3)
	if got, want := st.Loco(3), sim.NewState().Loco(3); got != want {
		t.Fatalf("Loco(3) after purge = %+v, want the default %+v", got, want)
	}
}

// withFunctions returns base with the functions of f.
func withFunctions(base, f protocol.LocoInfo) protocol.LocoInfo {
	base.Headlight = f.Headlight
	base.FunctionsF1F4 = f.FunctionsF1F4
	base.FunctionsF5F12 = f.FunctionsF5F12
	base.FunctionsF13F20 = f.FunctionsF13F20
	base.FunctionsF21F28 = f.FunctionsF21F28
	base.FunctionsF29F31 = f.FunctionsF29F31
	return base
}
