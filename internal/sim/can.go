package sim

import (
	"errors"
	"fmt"
	"slices"

	"github.com/trains-io/z21.go/protocol"
	"github.com/trains-io/z21.go/server"
)

const (
	defaultDetectorOccupancy = 0x0100 // free with voltage
	defaultBoosterVCCmV      = 10000
	defaultBoosterCurrentmA  = 100

	// maxCANDetectorPorts follows from the one-byte port in LAN_CAN_DETECTOR.
	maxCANDetectorPorts = 256
	// maxCANDeviceNameLen leaves room for the NUL terminator of the name field.
	maxCANDeviceNameLen = protocol.CANBoosterNameLen - 1
)

// ErrInvalidCANDevice is returned by BuildCANDevice for invalid parameters.
var ErrInvalidCANDevice = errors.New("z21-sim: invalid CAN device")

// CANDeviceKind identifies a simulated zLink CAN device.
type CANDeviceKind int

const (
	CANDeviceDetector CANDeviceKind = iota + 1
	CANDeviceBooster
)

// CANDevice is a simulated CAN occupancy detector or booster.
type CANDevice struct {
	Kind       CANDeviceKind
	NetID      uint16
	Name       string
	ModuleAddr uint16
	Ports      []uint16 // occupancy Value1 per port (detector)
	OutputPort uint16
	State      uint16
	VCCmV      uint16
	CurrentmA  uint16
}

func (d CANDevice) clone() CANDevice {
	d.Ports = slices.Clone(d.Ports)
	return d
}

func newDetectorDevice(netID uint16, name string, moduleAddr, portCount uint16) CANDevice {
	if moduleAddr == 0 {
		moduleAddr = 1
	}
	if portCount == 0 {
		portCount = 1
	}
	ports := make([]uint16, portCount)
	for i := range ports {
		ports[i] = defaultDetectorOccupancy
	}
	return CANDevice{
		Kind:       CANDeviceDetector,
		NetID:      netID,
		Name:       name,
		ModuleAddr: moduleAddr,
		Ports:      ports,
	}
}

func newBoosterDevice(netID uint16, name string, outputPort uint16) CANDevice {
	if outputPort == 0 {
		outputPort = 1
	}
	return CANDevice{
		Kind:       CANDeviceBooster,
		NetID:      netID,
		Name:       name,
		OutputPort: outputPort,
		VCCmV:      defaultBoosterVCCmV,
		CurrentmA:  defaultBoosterCurrentmA,
	}
}

// BuildCANDevice constructs a CAN device from control-plane parameters.
// Invalid parameters give an error wrapping ErrInvalidCANDevice.
func BuildCANDevice(netID uint32, kind CANDeviceKind, name string, moduleAddr, portCount, outputPort uint32) (CANDevice, error) {
	if netID == 0 || netID > 0xFFFF {
		return CANDevice{}, fmt.Errorf("%w: net_id %#x out of range [0x1, 0xffff]", ErrInvalidCANDevice, netID)
	}
	if len(name) > maxCANDeviceNameLen {
		return CANDevice{}, fmt.Errorf("%w: name %q longer than %d bytes", ErrInvalidCANDevice, name, maxCANDeviceNameLen)
	}
	switch kind {
	case CANDeviceDetector:
		if moduleAddr > 0xFFFF {
			return CANDevice{}, fmt.Errorf("%w: module_addr %d out of range [0, 65535]", ErrInvalidCANDevice, moduleAddr)
		}
		if portCount > maxCANDetectorPorts {
			return CANDevice{}, fmt.Errorf("%w: port_count %d out of range [0, %d]", ErrInvalidCANDevice, portCount, maxCANDetectorPorts)
		}
		return newDetectorDevice(uint16(netID), name, uint16(moduleAddr), uint16(portCount)), nil
	case CANDeviceBooster:
		if outputPort > 0xFFFF {
			return CANDevice{}, fmt.Errorf("%w: output_port %d out of range [0, 65535]", ErrInvalidCANDevice, outputPort)
		}
		return newBoosterDevice(uint16(netID), name, uint16(outputPort)), nil
	default:
		return CANDevice{}, fmt.Errorf("%w: unsupported kind", ErrInvalidCANDevice)
	}
}

// NotifyCANDeviceDetected adds device and broadcasts its initial status. It
// returns the number of clients notified. Used by the control API.
func (s *Server) NotifyCANDeviceDetected(device CANDevice) int {
	return s.broadcast(s.state.AddCANDevice(device))
}

func (s *Server) serveCAN(w server.ResponseWriter, r *server.Request) {
	replies, ok := s.handleCAN(r.Message)
	if !ok {
		s.log.Debug("unhandled CAN", "from", r.Client.Addr, "header", protocol.HeaderName(r.Message.Header))
		return
	}
	w.Reply(replies...)
}

func (s *Server) handleCAN(msg protocol.Message) ([]protocol.Message, bool) {
	switch msg.Header {
	case protocol.HeaderLANCANDeviceGetDescription:
		netID, err := protocol.ParseGetCANDeviceDescription(msg.Data)
		if err != nil {
			return nil, false
		}
		device, _ := s.state.CANDevice(netID)
		return []protocol.Message{protocol.EncodeCANDeviceDescription(netID, device.Name)}, true

	case protocol.HeaderLANCANDeviceSetDescription:
		netID, name, err := protocol.ParseSetCANDeviceDescription(msg.Data)
		if err != nil {
			return nil, false
		}
		s.state.SetCANDeviceName(netID, name)
		return nil, true

	case protocol.HeaderLANCANDetector:
		netID, err := protocol.ParseGetCANDetector(msg.Data)
		if err != nil {
			return nil, false
		}
		var reports []protocol.Message
		for _, device := range s.polledDetectors(netID) {
			reports = append(reports, canDetectorReports(device)...)
		}
		return reports, true

	case protocol.HeaderLANCANMaintenance:
		if netID, moduleAddr, _, err := protocol.ParseCANMaintenanceSetAddress(msg.Data); err == nil {
			s.state.SetCANDetectorAddr(netID, moduleAddr)
		}
		return nil, true

	case protocol.HeaderLANCANBoosterSetTrackPower:
		netID, power, err := protocol.ParseSetCANBoosterTrackPower(msg.Data)
		if err != nil {
			return nil, false
		}
		device, ok := s.state.SetCANBoosterTrackPower(netID, power)
		if !ok {
			return nil, true
		}
		return []protocol.Message{canBoosterState(device)}, true

	default:
		return nil, false
	}
}

// polledDetectors returns the detectors a LAN_CAN_DETECTOR poll for netID
// asks about.
func (s *Server) polledDetectors(netID uint16) []CANDevice {
	if netID == protocol.CANDetectorPollAll {
		return s.state.CANDetectors()
	}
	if device, ok := s.state.CANDevice(netID); ok && device.Kind == CANDeviceDetector {
		return []CANDevice{device}
	}
	return nil
}
