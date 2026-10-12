package sim

import (
	"github.com/trains-io/z21.go/protocol"
	"github.com/trains-io/z21.go/server"
)

// notify sends the changes caused by a client request. Other subscribers get
// them as broadcasts; the requester gets the ones it is subscribed to in its
// reply, so they cannot arrive before the reply.
func notify(w server.ResponseWriter, c server.Client, changes []Change) {
	for _, ch := range changes {
		w.Broadcast(ch.Flag, ch.Messages...)
		if c.BroadcastFlags&ch.Flag != 0 {
			w.Reply(ch.Messages...)
		}
	}
}

// broadcast sends changes that do not come from a client request, for example
// from the control API, to all subscribers. It returns the number of packets
// sent, which is the number of clients notified for a single change.
func (s *Server) broadcast(changes []Change) int {
	sent := 0
	for _, ch := range changes {
		sent += s.z21.Broadcast(ch.Flag, ch.Messages...)
	}
	return sent
}

// BroadcastSystemState sends LAN_SYSTEMSTATE_DATACHANGED to all clients
// subscribed to system-state broadcasts.
func (s *Server) BroadcastSystemState() int {
	return s.z21.Broadcast(protocol.BroadcastFlagSystemState, s.systemStateReply())
}

// SetTrackPowerAndNotify switches track power and, if that changed the state,
// broadcasts the change to all subscribed clients. Used by the control API.
func (s *Server) SetTrackPowerAndNotify(on bool) bool {
	changes := s.state.SetTrackPower(on)
	s.broadcast(changes)
	return len(changes) > 0
}
