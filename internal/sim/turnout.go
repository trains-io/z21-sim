package sim

import (
	"fmt"

	"github.com/trains-io/z21.go/protocol"
	"github.com/trains-io/z21.go/server"
)

const (
	xHeaderGetTurnoutInfo  byte = 0x43
	xHeaderSetTurnout      byte = 0x53
	xHeaderGetExtAccessory byte = 0x44
	xHeaderSetExtAccessory byte = 0x54
)

// turnoutInfoHandler answers LAN_X_GET_TURNOUT_INFO (spec §5.1).
func (s *Server) turnoutInfoHandler() server.Handler {
	return server.HandlerFunc(func(w server.ResponseWriter, r *server.Request) {
		addr, err := protocol.ParseGetTurnoutInfo(r.Message.Data)
		if err == nil {
			err = checkAccessoryAddress(addr)
		}
		if err != nil {
			w.Reply(protocol.EncodeUnknownCommand())
			return
		}
		w.Reply(protocol.EncodeTurnoutInfo(s.state.Turnout(addr)))
	})
}

// setTurnoutHandler handles LAN_X_SET_TURNOUT (spec §5.2). There is no reply;
// X-Bus subscribers get LAN_X_TURNOUT_INFO, the requester too when
// subscribed.
func (s *Server) setTurnoutHandler() server.Handler {
	return accessoryHandler(func(data []byte) ([]Change, error) {
		addr, sw, err := protocol.ParseSetTurnout(data)
		if err != nil {
			return nil, err
		}
		_, changes, err := s.state.SwitchTurnout(addr, sw)
		return changes, err
	})
}

// extAccessoryInfoHandler answers LAN_X_GET_EXT_ACCESSORY_INFO (spec §5.5).
func (s *Server) extAccessoryInfoHandler() server.Handler {
	return server.HandlerFunc(func(w server.ResponseWriter, r *server.Request) {
		addr, err := protocol.ParseGetExtAccessoryInfo(r.Message.Data)
		if err == nil {
			err = checkAccessoryAddress(addr)
		}
		if err != nil {
			w.Reply(protocol.EncodeUnknownCommand())
			return
		}
		w.Reply(protocol.EncodeExtAccessoryInfo(s.state.ExtAccessory(addr)))
	})
}

// setExtAccessoryHandler handles LAN_X_SET_EXT_ACCESSORY (spec §5.4). There
// is no reply; X-Bus subscribers get LAN_X_EXT_ACCESSORY_INFO, the requester
// too when subscribed.
func (s *Server) setExtAccessoryHandler() server.Handler {
	return accessoryHandler(func(data []byte) ([]Change, error) {
		addr, value, err := protocol.ParseSetExtAccessory(data)
		if err != nil {
			return nil, err
		}
		_, changes, err := s.state.SetExtAccessory(addr, value)
		return changes, err
	})
}

// accessoryHandler adapts a turnout or accessory command to a Handler.
// Malformed requests and addresses out of range get LAN_X_UNKNOWN_COMMAND.
func accessoryHandler(apply func(data []byte) ([]Change, error)) server.Handler {
	return server.HandlerFunc(func(w server.ResponseWriter, r *server.Request) {
		changes, err := apply(r.Message.Data)
		if err != nil {
			w.Reply(protocol.EncodeUnknownCommand())
			return
		}
		notify(w, r.Client, changes)
	})
}

// SetTurnout switches addr to pos (protocol.TurnoutPositionOutput1 or
// Output2), as a throttle outside the Z21 LAN would, and sends
// LAN_X_TURNOUT_INFO to the X-Bus subscribers, also when the position stays
// the same. It returns the turnout and how many clients were notified, or an
// error wrapping ErrInvalidAccessory. Used by the control API.
func (s *Server) SetTurnout(addr uint16, pos protocol.TurnoutPosition) (protocol.TurnoutInfo, int, error) {
	var sw protocol.TurnoutSwitch
	switch pos {
	case protocol.TurnoutPositionOutput1:
		sw = protocol.TurnoutSwitch{Activate: true}
	case protocol.TurnoutPositionOutput2:
		sw = protocol.TurnoutSwitch{Activate: true, Output2: true}
	default:
		return protocol.TurnoutInfo{}, 0, fmt.Errorf("%w: turnout position %#02x, want output 1 or 2", ErrInvalidAccessory, byte(pos))
	}
	info, changes, err := s.state.SwitchTurnout(addr, sw)
	if err != nil {
		return protocol.TurnoutInfo{}, 0, err
	}
	return info, s.broadcast(changes), nil
}

// SetExtAccessory sets addr to value, as a throttle outside the Z21 LAN would,
// and sends LAN_X_EXT_ACCESSORY_INFO to the X-Bus subscribers, also when the
// value stays the same. It returns the accessory and how many clients were
// notified, or an error wrapping ErrInvalidAccessory. Used by the control API.
func (s *Server) SetExtAccessory(addr uint16, value byte) (protocol.ExtAccessoryInfo, int, error) {
	info, changes, err := s.state.SetExtAccessory(addr, value)
	if err != nil {
		return protocol.ExtAccessoryInfo{}, 0, err
	}
	return info, s.broadcast(changes), nil
}
