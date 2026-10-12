package sim

import (
	"cmp"
	"slices"

	"github.com/trains-io/z21.go/protocol"
)

// AddCANDevice adds or replaces a CAN device. Clients learn about it as about a
// newly detected device: detector subscribers get its occupancy reports,
// booster subscribers its system state.
func (st *State) AddCANDevice(device CANDevice) []Change {
	device = device.clone()
	st.mu.Lock()
	st.canDevices[device.NetID] = device
	st.mu.Unlock()

	switch device.Kind {
	case CANDeviceDetector:
		return []Change{{Flag: protocol.BroadcastFlagCANDetector, Messages: canDetectorReports(device)}}
	case CANDeviceBooster:
		return []Change{{Flag: protocol.BroadcastFlagCANBooster, Messages: []protocol.Message{canBoosterState(device)}}}
	default:
		return nil
	}
}

// CANDevice returns the CAN device with netID.
func (st *State) CANDevice(netID uint16) (CANDevice, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	device, ok := st.canDevices[netID]
	return device.clone(), ok
}

// CANDetectors returns all CAN occupancy detectors, ordered by NetID.
func (st *State) CANDetectors() []CANDevice {
	st.mu.Lock()
	var out []CANDevice
	for _, device := range st.canDevices {
		if device.Kind == CANDeviceDetector {
			out = append(out, device.clone())
		}
	}
	st.mu.Unlock()
	slices.SortFunc(out, func(a, b CANDevice) int { return cmp.Compare(a.NetID, b.NetID) })
	return out
}

// SetCANDeviceName sets the description of netID, adding an unknown device
// without a kind if there is none, as a real Z21 stores any description.
func (st *State) SetCANDeviceName(netID uint16, name string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	device, ok := st.canDevices[netID]
	if !ok {
		device = CANDevice{NetID: netID}
	}
	device.Name = name
	st.canDevices[netID] = device
}

// SetCANDetectorAddr sets the module address of the detector netID. Other
// devices are left unchanged.
func (st *State) SetCANDetectorAddr(netID, moduleAddr uint16) {
	st.mu.Lock()
	defer st.mu.Unlock()
	device, ok := st.canDevices[netID]
	if !ok || device.Kind != CANDeviceDetector {
		return
	}
	device.ModuleAddr = moduleAddr
	st.canDevices[netID] = device
}

// SetCANBoosterTrackPower switches the track voltage of the booster netID
// and returns the updated booster. It reports false if netID is not a booster.
func (st *State) SetCANBoosterTrackPower(netID uint16, power protocol.CANBoosterTrackPower) (CANDevice, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	device, ok := st.canDevices[netID]
	if !ok || device.Kind != CANDeviceBooster {
		return CANDevice{}, false
	}
	switch power {
	case protocol.CANBoosterTrackPowerActivateAll, protocol.CANBoosterTrackPowerActivateOut1, protocol.CANBoosterTrackPowerActivateOut2:
		device.State &^= protocol.CANBoosterStateTrackVoltageOff
	case protocol.CANBoosterTrackPowerDeactivateAll, protocol.CANBoosterTrackPowerDeactivateOut1, protocol.CANBoosterTrackPowerDeactivateOut2:
		device.State |= protocol.CANBoosterStateTrackVoltageOff
	}
	st.canDevices[netID] = device
	return device, true
}

func canDetectorReports(device CANDevice) []protocol.Message {
	reports := make([]protocol.Message, 0, len(device.Ports))
	for port, value := range device.Ports {
		reports = append(reports, protocol.EncodeCANDetector(protocol.CANDetectorReport{
			NetID:  device.NetID,
			Addr:   device.ModuleAddr,
			Port:   uint8(port),
			Type:   protocol.CANDetectorTypeOccupancy,
			Value1: value,
		}))
	}
	return reports
}

func canBoosterState(device CANDevice) protocol.Message {
	return protocol.EncodeCANBoosterSystemState(protocol.CANBoosterSystemState{
		NetID:      device.NetID,
		OutputPort: device.OutputPort,
		State:      device.State,
		VCCVoltage: device.VCCmV,
		Current:    device.CurrentmA,
	})
}
