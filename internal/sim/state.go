package sim

import (
	"sync"

	"github.com/trains-io/z21.go/protocol"
)

const (
	defaultSupplyVoltageMV = 10000
	defaultVCCVoltageMV    = 1000
	defaultMainCurrentMA   = 100
	defaultTemperatureC    = 42
	defaultCapabilities    = protocol.CapabilityDCC | protocol.CapabilityLocoCmds | protocol.CapabilityAccessoryCmds
)

// Change is a broadcast caused by a state change: Messages go to the clients
// subscribed to Flag (protocol.BroadcastFlag*).
type Change struct {
	Flag     uint32
	Messages []protocol.Message
}

// State is the simulated command station state. UDP handlers and the control
// API change it through its methods, which return the broadcasts the change
// causes, so both produce the same broadcasts. It is safe for concurrent use.
type State struct {
	mu sync.Mutex
	// system holds the values measured while the track is live; clients see
	// them through reportedLocked.
	system     protocol.SystemState
	canDevices map[uint16]CANDevice
}

// NewState returns the state of a command station that has just been powered
// on: track power on, nominal currents and voltages, no CAN devices.
func NewState() *State {
	return &State{
		system: protocol.SystemState{
			MainCurrent:         defaultMainCurrentMA,
			FilteredMainCurrent: defaultMainCurrentMA - 20,
			Temperature:         defaultTemperatureC,
			SupplyVoltage:       defaultSupplyVoltageMV,
			VCCVoltage:          defaultVCCVoltageMV,
			Capabilities:        defaultCapabilities,
		},
		canDevices: make(map[uint16]CANDevice),
	}
}

// System returns the system state as reported by LAN_SYSTEMSTATE_DATACHANGED.
// While the track voltage is off, the currents and the VCC (track) voltage
// are reported as zero.
func (st *State) System() protocol.SystemState {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.reportedLocked()
}

func (st *State) reportedLocked() protocol.SystemState {
	s := st.system
	if s.CentralState&protocol.CentralStateTrackVoltageOff != 0 {
		s.MainCurrent, s.ProgCurrent, s.FilteredMainCurrent = 0, 0, 0
		s.VCCVoltage = 0
	}
	return s
}

// TrackPower reports whether the track voltage is on.
func (st *State) TrackPower() bool {
	return st.System().CentralState&protocol.CentralStateTrackVoltageOff == 0
}

// SystemValues are the measured values of the system state (spec §2.18). Nil
// fields are left unchanged.
type SystemValues struct {
	MainCurrent         *int16  // mA
	ProgCurrent         *int16  // mA
	FilteredMainCurrent *int16  // mA
	Temperature         *int16  // °C
	SupplyVoltage       *uint16 // mV
	VCCVoltage          *uint16 // mV
}

// SetSystemValues changes the measured values. If that changes the reported
// state, system-state subscribers get the new system state. While the track
// voltage is off, currents and VCC voltage are kept and reported once the
// track voltage is back on.
func (st *State) SetSystemValues(v SystemValues) []Change {
	st.mu.Lock()
	before := st.reportedLocked()
	setIfNotNil(&st.system.MainCurrent, v.MainCurrent)
	setIfNotNil(&st.system.ProgCurrent, v.ProgCurrent)
	setIfNotNil(&st.system.FilteredMainCurrent, v.FilteredMainCurrent)
	setIfNotNil(&st.system.Temperature, v.Temperature)
	setIfNotNil(&st.system.SupplyVoltage, v.SupplyVoltage)
	setIfNotNil(&st.system.VCCVoltage, v.VCCVoltage)
	system := st.reportedLocked()
	st.mu.Unlock()

	if system == before {
		return nil
	}
	return []Change{systemStateChange(system)}
}

func setIfNotNil[T any](dst *T, v *T) {
	if v != nil {
		*dst = *v
	}
}

func systemStateChange(system protocol.SystemState) Change {
	return Change{Flag: protocol.BroadcastFlagSystemState, Messages: []protocol.Message{protocol.EncodeSystemState(system)}}
}

// trackPowerOnClears are the central-state flags that switching track power
// on resets, as in ZIMO's Z21 server.
const trackPowerOnClears = protocol.CentralStateEmergencyStop |
	protocol.CentralStateTrackVoltageOff |
	protocol.CentralStateShortCircuit |
	protocol.CentralStateProgrammingMode

// EmergencyStopped reports whether an emergency stop is active.
func (st *State) EmergencyStopped() bool {
	return st.System().CentralState&protocol.CentralStateEmergencyStop != 0
}

// SetEmergencyStop stops all locos; the track voltage stays as it is.
// Switching track power on ends the emergency stop. If that changes the
// state, X-Bus subscribers get LAN_X_BC_STOPPED and the X status, and
// system-state subscribers the new system state.
func (st *State) SetEmergencyStop() []Change {
	st.mu.Lock()
	before := st.system.CentralState
	st.system.CentralState |= protocol.CentralStateEmergencyStop
	system := st.reportedLocked()
	st.mu.Unlock()

	if system.CentralState == before {
		return nil
	}
	return []Change{
		{Flag: protocol.BroadcastFlagXpressNet, Messages: []protocol.Message{
			protocol.EncodeBCStopped(),
			protocol.EncodeXStatus(protocol.XStatus{CentralState: system.CentralState}),
		}},
		systemStateChange(system),
	}
}

// SetTrackPower switches the track voltage. Switching it on also ends an
// emergency stop, a short circuit and programming mode; switching it off
// keeps an emergency stop. If that changes the state, X-Bus subscribers are
// told the new track power and X status, and system-state subscribers the
// new system state.
func (st *State) SetTrackPower(on bool) []Change {
	st.mu.Lock()
	before := st.system.CentralState
	if on {
		st.system.CentralState &^= trackPowerOnClears
	} else {
		st.system.CentralState |= protocol.CentralStateTrackVoltageOff
	}
	system := st.reportedLocked()
	st.mu.Unlock()

	if system.CentralState == before {
		return nil
	}
	return []Change{
		{Flag: protocol.BroadcastFlagXpressNet, Messages: []protocol.Message{
			protocol.EncodeTrackPowerBC(on),
			protocol.EncodeXStatus(protocol.XStatus{CentralState: system.CentralState}),
		}},
		systemStateChange(system),
	}
}
