package server

import (
	"encoding/binary"

	"github.com/trains-io/z21.go/protocol"
)

func (s *Server) broadcastFlagsReply(flags uint32) protocol.Message {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, flags)
	return protocol.Message{Header: protocol.HeaderLANGetBroadcastFlags, Data: data}
}

func (s *Server) clientsSubscribed(flag uint32) []*clientSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []*clientSession
	for _, c := range s.clients {
		if c.broadcastFlags&flag != 0 {
			out = append(out, c)
		}
	}
	return out
}
