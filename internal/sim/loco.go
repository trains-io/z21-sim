package sim

import (
	"github.com/trains-io/z21.go/protocol"
	"github.com/trains-io/z21.go/server"
)

const (
	xHeaderGetLocoInfo      byte = 0xE3
	xCommandGetLocoInfo     byte = 0xF0
	xCommandPurgeLoco       byte = 0x44
	xHeaderSetLocoDrive     byte = 0xE4
	xCommandSetLocoFunction byte = 0xF8
	xHeaderSetLocoBinary    byte = 0xE5
	xCommandSetLocoBinary   byte = 0x5F
	xHeaderSetLocoEStop     byte = 0x92
)

// locoFunctionGroups are the DB0 values of LAN_X_SET_LOCO_FUNCTION_GROUP.
var locoFunctionGroups = []protocol.LocoFunctionGroup{
	protocol.LocoFunctionGroupF0F4, protocol.LocoFunctionGroupF5F8,
	protocol.LocoFunctionGroupF9F12, protocol.LocoFunctionGroupF13F20,
	protocol.LocoFunctionGroupF21F28, protocol.LocoFunctionGroupF29F36,
	protocol.LocoFunctionGroupF37F44, protocol.LocoFunctionGroupF45F52,
	protocol.LocoFunctionGroupF53F60, protocol.LocoFunctionGroupF61F68,
}

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

// locoDriveHandler handles LAN_X_SET_LOCO_DRIVE (spec §4.2).
func (s *Server) locoDriveHandler() server.Handler {
	return s.locoHandler(func(data []byte) (protocol.LocoInfo, bool, error) {
		drive, err := protocol.ParseSetLocoDrive(data)
		if err != nil {
			return protocol.LocoInfo{}, false, err
		}
		info, changed := s.state.DriveLoco(drive)
		return info, changed, nil
	})
}

// locoFunctionHandler handles LAN_X_SET_LOCO_FUNCTION (spec §4.3.1).
func (s *Server) locoFunctionHandler() server.Handler {
	return s.locoHandler(func(data []byte) (protocol.LocoInfo, bool, error) {
		addr, fn, action, err := protocol.ParseSetLocoFunction(data)
		if err != nil {
			return protocol.LocoInfo{}, false, err
		}
		info, changed := s.state.SetLocoFunction(addr, fn, action)
		return info, changed, nil
	})
}

// locoFunctionGroupHandler handles LAN_X_SET_LOCO_FUNCTION_GROUP (spec §4.3.2).
func (s *Server) locoFunctionGroupHandler() server.Handler {
	return s.locoHandler(func(data []byte) (protocol.LocoInfo, bool, error) {
		addr, group, functions, err := protocol.ParseSetLocoFunctionGroup(data)
		if err != nil {
			return protocol.LocoInfo{}, false, err
		}
		info, changed := s.state.SetLocoFunctionGroup(addr, group, functions)
		return info, changed, nil
	})
}

// locoEStopHandler handles LAN_X_SET_LOCO_E_STOP (spec §4.5).
func (s *Server) locoEStopHandler() server.Handler {
	return s.locoHandler(func(data []byte) (protocol.LocoInfo, bool, error) {
		addr, err := protocol.ParseSetLocoEStop(data)
		if err != nil {
			return protocol.LocoInfo{}, false, err
		}
		info, changed := s.state.EStopLoco(addr)
		return info, changed, nil
	})
}

// locoHandler adapts a loco command to a Handler. Loco commands get no reply;
// if the loco changed, its subscribers get LAN_X_LOCO_INFO, the requester too
// when subscribed. Malformed requests get LAN_X_UNKNOWN_COMMAND.
func (s *Server) locoHandler(apply func(data []byte) (protocol.LocoInfo, bool, error)) server.Handler {
	return server.HandlerFunc(func(w server.ResponseWriter, r *server.Request) {
		info, changed, err := apply(r.Message.Data)
		if err != nil {
			w.Reply(protocol.EncodeUnknownCommand())
			return
		}
		if changed {
			w.BroadcastLocoInfo(info)
		}
	})
}

// purgeLocoHandler handles LAN_X_PURGE_LOCO (spec §4.6): the loco is
// forgotten, nobody is notified.
func (s *Server) purgeLocoHandler() server.Handler {
	return server.HandlerFunc(func(w server.ResponseWriter, r *server.Request) {
		addr, err := protocol.ParsePurgeLoco(r.Message.Data)
		if err != nil {
			w.Reply(protocol.EncodeUnknownCommand())
			return
		}
		s.state.PurgeLoco(addr)
	})
}

// locoBinaryStateHandler accepts LAN_X_SET_LOCO_BINARY_STATE (spec §4.3.3)
// without effect, as ZIMO's simulator: LAN_X_LOCO_INFO does not report
// binary states.
func locoBinaryStateHandler() server.Handler {
	return server.HandlerFunc(func(server.ResponseWriter, *server.Request) {})
}
