package sim

import (
	"github.com/trains-io/z21.go/protocol"
	"github.com/trains-io/z21.go/server"
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

// trackPowerHandler switches track power. The requester gets the track power
// broadcast as reply; other X-Bus subscribers get it with the new X status, and
// system-state subscribers get the new system state. A requester subscribed to
// system state gets it in its reply packet, so it arrives with the reply.
func (s *Server) trackPowerHandler(on bool) server.Handler {
	return server.HandlerFunc(func(w server.ResponseWriter, r *server.Request) {
		w.Reply(protocol.EncodeTrackPowerBC(on))
		if !s.setTrackPower(on) {
			return
		}
		w.Broadcast(protocol.BroadcastFlagXpressNet, protocol.EncodeTrackPowerBC(on), s.xStatusReply())
		state := s.systemStateReply()
		w.Broadcast(protocol.BroadcastFlagSystemState, state)
		if r.Client.BroadcastFlags&protocol.BroadcastFlagSystemState != 0 {
			w.Reply(state)
		}
	})
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
