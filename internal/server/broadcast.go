package server

import (
	"encoding/binary"
	"net"

	"github.com/trains-io/z21.go/protocol"
)

func (s *Server) broadcastFlagsReply(flags uint32) protocol.Message {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, flags)
	return protocol.Message{Header: protocol.HeaderLANGetBroadcastFlags, Data: data}
}

// BroadcastSystemState sends LAN_SYSTEMSTATE_DATACHANGED to all clients
// subscribed to system-state broadcasts.
func (s *Server) BroadcastSystemState() int {
	return s.broadcast(s.systemStateReply(), protocol.BroadcastFlagSystemState, nil)
}

// SetTrackPowerAndNotify updates track power and emits unsolicited broadcasts
// to all subscribed clients. Used by the gRPC control plane.
func (s *Server) SetTrackPowerAndNotify(on bool) bool {
	if !s.setTrackPower(on) {
		return false
	}
	s.notifyTrackPowerChange(on, nil)
	return true
}

func (s *Server) notifyTrackPowerChange(on bool, except *net.UDPAddr) {
	s.broadcastMany(
		[]protocol.Message{s.trackPowerBCReply(on), s.xStatusReply()},
		protocol.BroadcastFlagXpressNet,
		except,
	)
	s.broadcast(s.systemStateReply(), protocol.BroadcastFlagSystemState, nil)
}

func (s *Server) broadcast(msg protocol.Message, flag uint32, except *net.UDPAddr) int {
	return s.broadcastMany([]protocol.Message{msg}, flag, except)
}

func (s *Server) broadcastMany(msgs []protocol.Message, flag uint32, except *net.UDPAddr) int {
	if len(msgs) == 0 {
		return 0
	}

	sent := 0
	for _, addr := range s.subscriberAddrs(flag) {
		if except != nil && addr.String() == except.String() {
			continue
		}
		s.writeMessages(addr, msgs)
		sent++
	}
	return sent
}

func (s *Server) subscriberAddrs(flag uint32) []*net.UDPAddr {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []*net.UDPAddr
	for _, c := range s.clients {
		if c.broadcastFlags&flag != 0 && c.addr != nil {
			out = append(out, c.addr)
		}
	}
	return out
}
