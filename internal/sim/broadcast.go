package sim

import (
	"errors"
	"fmt"

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

// ErrInvalidInjection is returned by InjectRaw for a request it cannot send.
var ErrInvalidInjection = errors.New("z21-sim: invalid injection")

// InjectRaw sends the datasets in payload, as one packet, to the clients
// subscribed to any bit of flag (protocol.BroadcastFlag*), without changing the
// simulator state. It returns the number of clients it was sent to.
func (s *Server) InjectRaw(flag uint32, payload []byte) (int, error) {
	if flag == 0 {
		return 0, fmt.Errorf("%w: broadcast flags must not be zero", ErrInvalidInjection)
	}
	msgs, err := protocol.ParseAll(payload)
	if err != nil {
		return 0, fmt.Errorf("%w: payload: %v", ErrInvalidInjection, err)
	}
	if len(msgs) == 0 {
		return 0, fmt.Errorf("%w: payload has no datasets", ErrInvalidInjection)
	}
	return s.z21.Broadcast(flag, msgs...), nil
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
