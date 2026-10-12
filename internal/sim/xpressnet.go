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
	xHeaderSetStop           byte = 0x80
	xCommandGetFirmware      byte = 0xF1
	xFirmwareDB0             byte = 0x0A
	defaultXFirmwareMSB      byte = 0x01
	defaultXFirmwareLSB      byte = 0x20 // BCD 1.20
)

// trackPowerHandler switches track power; the requester always gets the track
// power broadcast.
func (s *Server) trackPowerHandler(on bool) server.Handler {
	return server.HandlerFunc(func(w server.ResponseWriter, r *server.Request) {
		confirmAndNotify(w, r.Client, s.state.SetTrackPower(on), protocol.EncodeTrackPowerBC(on))
	})
}

// stopHandler handles LAN_X_SET_STOP; the requester always gets
// LAN_X_BC_STOPPED.
func (s *Server) stopHandler() server.Handler {
	return server.HandlerFunc(func(w server.ResponseWriter, r *server.Request) {
		confirmAndNotify(w, r.Client, s.state.SetEmergencyStop(), protocol.EncodeBCStopped())
	})
}

// confirmAndNotify sends the X-Bus broadcast confirm, which the changes carry
// when the state changed, to the requester even when it is not an X-Bus
// subscriber or nothing changed, and notifies the subscribers of the changes.
func confirmAndNotify(w server.ResponseWriter, c server.Client, changes []Change, confirm protocol.Message) {
	if len(changes) == 0 || c.BroadcastFlags&protocol.BroadcastFlagXpressNet == 0 {
		w.Reply(confirm)
	}
	notify(w, c, changes)
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
