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
