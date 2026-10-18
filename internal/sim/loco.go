package sim

import (
	"github.com/trains-io/z21.go/protocol"
	"github.com/trains-io/z21.go/server"
)

const (
	xHeaderGetLocoInfo  byte = 0xE3
	xCommandGetLocoInfo byte = 0xF0
	xHeaderSetLocoDrive byte = 0xE4
)

// locoInfoHandler answers LAN_X_GET_LOCO_INFO. z21.go/server has already
// subscribed the requester to the loco.
func (s *Server) locoInfoHandler() server.Handler {
	return server.HandlerFunc(func(w server.ResponseWriter, r *server.Request) {
		addr, err := protocol.ParseGetLocoInfo(r.Message.Data)
		if err != nil {
			w.Reply(protocol.EncodeUnknownCommand())
			return
		}
		w.Reply(protocol.EncodeLocoInfo(s.state.Loco(addr)))
	})
}

// locoDriveHandler handles LAN_X_SET_LOCO_DRIVE. There is no reply; if the
// loco changed, its subscribers get LAN_X_LOCO_INFO, the requester too when
// subscribed (spec §4.2).
func (s *Server) locoDriveHandler() server.Handler {
	return server.HandlerFunc(func(w server.ResponseWriter, r *server.Request) {
		drive, err := protocol.ParseSetLocoDrive(r.Message.Data)
		if err != nil {
			w.Reply(protocol.EncodeUnknownCommand())
			return
		}
		if info, changed := s.state.DriveLoco(drive); changed {
			w.BroadcastLocoInfo(info)
		}
	})
}
