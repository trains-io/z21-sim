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

// trackPowerHandler switches track power. The requester always gets the track
// power broadcast as reply: through notify when the state changed and it is an
// X-Bus subscriber, directly otherwise.
func (s *Server) trackPowerHandler(on bool) server.Handler {
	return server.HandlerFunc(func(w server.ResponseWriter, r *server.Request) {
		changes := s.state.SetTrackPower(on)
		if len(changes) == 0 || r.Client.BroadcastFlags&protocol.BroadcastFlagXpressNet == 0 {
			w.Reply(protocol.EncodeTrackPowerBC(on))
		}
		notify(w, r.Client, changes)
	})
}

func (s *Server) xFirmwareReply() protocol.Message {
	return protocol.EncodeXFirmware(protocol.XFirmware{VersionMSB: defaultXFirmwareMSB, VersionLSB: defaultXFirmwareLSB})
}

func (s *Server) xStatusReply() protocol.Message {
	return protocol.EncodeXStatus(protocol.XStatus{CentralState: s.state.System().CentralState})
}

func (s *Server) systemStateReply() protocol.Message {
	return protocol.EncodeSystemState(s.state.System())
}
