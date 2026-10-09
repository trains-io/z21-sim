package sim

import (
	"net"

	"github.com/trains-io/z21.go/protocol"
)

const (
	xHeaderGetVersion        byte = 0x21
	xCommandGetStatus        byte = 0x24
	xCommandSetTrackPowerOff byte = 0x80
	xCommandSetTrackPowerOn  byte = 0x81
	xCommandGetFirmware      byte = 0xF1
	xFirmwareDB0             byte = 0x0A
	defaultXFirmwareMSB      byte = 0x01
	defaultXFirmwareLSB      byte = 0x20 // BCD 1.20
)

func (s *Server) handleLANX(remote *net.UDPAddr, msg protocol.Message) (protocol.Message, bool) {
	xHeader, data, err := protocol.ParseLANX(msg)
	if err != nil || len(data) < 1 {
		return protocol.Message{}, false
	}

	switch xHeader {
	case xCommandGetFirmware:
		if data[0] == xFirmwareDB0 {
			return s.xFirmwareReply(), true
		}
	case xHeaderGetVersion:
		switch data[0] {
		case xCommandGetStatus:
			return s.xStatusReply(), true
		case xCommandSetTrackPowerOff:
			if s.setTrackPower(false) {
				s.notifyTrackPowerChange(false, remote)
			}
			return protocol.EncodeTrackPowerBC(false), true
		case xCommandSetTrackPowerOn:
			if s.setTrackPower(true) {
				s.notifyTrackPowerChange(true, remote)
			}
			return protocol.EncodeTrackPowerBC(true), true
		}
	}

	return protocol.Message{}, false
}

func (s *Server) setTrackPower(on bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trackPower == on {
		return false
	}
	s.trackPower = on
	return true
}

func (s *Server) centralState() byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trackPower {
		return 0
	}
	return centralStateTrackVoltageOff
}

func (s *Server) xFirmwareReply() protocol.Message {
	return protocol.EncodeXFirmware(protocol.XFirmware{VersionMSB: defaultXFirmwareMSB, VersionLSB: defaultXFirmwareLSB})
}

func (s *Server) xStatusReply() protocol.Message {
	return protocol.EncodeXStatus(protocol.XStatus{CentralState: s.centralState()})
}
