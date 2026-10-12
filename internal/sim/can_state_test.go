package sim_test

import (
	"testing"

	"github.com/trains-io/z21-sim/internal/sim"
	"github.com/trains-io/z21.go/protocol"
)

func mustBuildCANDevice(t *testing.T, netID uint32, kind sim.CANDeviceKind, moduleAddr, portCount, outputPort uint32) sim.CANDevice {
	t.Helper()
	device, err := sim.BuildCANDevice(netID, kind, "dev", moduleAddr, portCount, outputPort)
	if err != nil {
		t.Fatalf("BuildCANDevice: %v", err)
	}
	return device
}

func TestStateAddCANDevice(t *testing.T) {
	tests := []struct {
		name     string
		device   sim.CANDevice
		flag     uint32
		messages int
		header   uint16
	}{
		{
			name:     "detector reports every port",
			device:   mustBuildCANDevice(t, 0xDB04, sim.CANDeviceDetector, 31, 3, 0),
			flag:     protocol.BroadcastFlagCANDetector,
			messages: 3,
			header:   protocol.HeaderLANCANDetector,
		},
		{
			name:     "booster reports its system state",
			device:   mustBuildCANDevice(t, 0xC101, sim.CANDeviceBooster, 0, 0, 1),
			flag:     protocol.BroadcastFlagCANBooster,
			messages: 1,
			header:   protocol.HeaderLANCANBoosterSystemState,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := sim.NewState()
			changes := st.AddCANDevice(tt.device)
			if len(changes) != 1 || changes[0].Flag != tt.flag || len(changes[0].Messages) != tt.messages {
				t.Fatalf("changes = %+v, want one change with flag %#x and %d messages", changes, tt.flag, tt.messages)
			}
			for _, msg := range changes[0].Messages {
				if msg.Header != tt.header {
					t.Fatalf("message header = %#04x, want %#04x", msg.Header, tt.header)
				}
			}
			if _, ok := st.CANDevice(tt.device.NetID); !ok {
				t.Fatalf("CANDevice(%#04x) not found after AddCANDevice", tt.device.NetID)
			}
		})
	}
}

func TestStateCANDeviceIsACopy(t *testing.T) {
	st := sim.NewState()
	device := mustBuildCANDevice(t, 0xDB04, sim.CANDeviceDetector, 1, 1, 0)
	st.AddCANDevice(device)
	device.Ports[0] = 0xFFFF

	got, _ := st.CANDevice(0xDB04)
	got.Ports[0] = 0xEEEE

	if again, _ := st.CANDevice(0xDB04); again.Ports[0] != 0x0100 {
		t.Fatalf("stored port value = %#04x, want unchanged 0x0100", again.Ports[0])
	}
}

func TestStateCANDetectorsOrderedByNetID(t *testing.T) {
	st := sim.NewState()
	for _, netID := range []uint32{0xDB09, 0xDB01, 0xDB05} {
		st.AddCANDevice(mustBuildCANDevice(t, netID, sim.CANDeviceDetector, 1, 1, 0))
	}
	st.AddCANDevice(mustBuildCANDevice(t, 0xC101, sim.CANDeviceBooster, 0, 0, 1))

	var got []uint16
	for _, device := range st.CANDetectors() {
		got = append(got, device.NetID)
	}
	if want := []uint16{0xDB01, 0xDB05, 0xDB09}; len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("CANDetectors NetIDs = %#04x, want %#04x", got, want)
	}
}

func TestStateSetCANDeviceName(t *testing.T) {
	st := sim.NewState()
	st.SetCANDeviceName(0x1234, "unknown device")

	device, ok := st.CANDevice(0x1234)
	if !ok || device.Name != "unknown device" || device.Kind != 0 {
		t.Fatalf("CANDevice = %+v, ok=%v; want a named device without kind", device, ok)
	}
}

func TestStateSetCANDetectorAddr(t *testing.T) {
	st := sim.NewState()
	st.AddCANDevice(mustBuildCANDevice(t, 0xDB04, sim.CANDeviceDetector, 1, 1, 0))
	st.AddCANDevice(mustBuildCANDevice(t, 0xC101, sim.CANDeviceBooster, 0, 0, 1))

	st.SetCANDetectorAddr(0xDB04, 42)
	st.SetCANDetectorAddr(0xC101, 42)

	if d, _ := st.CANDevice(0xDB04); d.ModuleAddr != 42 {
		t.Fatalf("detector ModuleAddr = %d, want 42", d.ModuleAddr)
	}
	if b, _ := st.CANDevice(0xC101); b.ModuleAddr != 0 {
		t.Fatalf("booster ModuleAddr = %d, want unchanged 0", b.ModuleAddr)
	}
}

func TestStateSetCANBoosterTrackPower(t *testing.T) {
	st := sim.NewState()
	st.AddCANDevice(mustBuildCANDevice(t, 0xC101, sim.CANDeviceBooster, 0, 0, 1))
	st.AddCANDevice(mustBuildCANDevice(t, 0xDB04, sim.CANDeviceDetector, 1, 1, 0))

	off, ok := st.SetCANBoosterTrackPower(0xC101, protocol.CANBoosterTrackPowerDeactivateAll)
	if !ok || off.State&protocol.CANBoosterStateTrackVoltageOff == 0 {
		t.Fatalf("after deactivate: %+v, ok=%v; want track voltage off", off, ok)
	}
	on, ok := st.SetCANBoosterTrackPower(0xC101, protocol.CANBoosterTrackPowerActivateAll)
	if !ok || on.State&protocol.CANBoosterStateTrackVoltageOff != 0 {
		t.Fatalf("after activate: %+v, ok=%v; want track voltage on", on, ok)
	}
	if _, ok := st.SetCANBoosterTrackPower(0xDB04, protocol.CANBoosterTrackPowerActivateAll); ok {
		t.Fatal("SetCANBoosterTrackPower on a detector reported ok")
	}
}
