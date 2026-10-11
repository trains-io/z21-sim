package sim

import (
	"github.com/trains-io/z21.go/protocol"
)

// BroadcastSystemState sends LAN_SYSTEMSTATE_DATACHANGED to all clients
// subscribed to system-state broadcasts.
func (s *Server) BroadcastSystemState() int {
	return s.z21.Broadcast(protocol.BroadcastFlagSystemState, s.systemStateReply())
}

// SetTrackPowerAndNotify updates track power and emits unsolicited broadcasts
// to all subscribed clients. Used by the gRPC control plane.
func (s *Server) SetTrackPowerAndNotify(on bool) bool {
	if !s.setTrackPower(on) {
		return false
	}
	s.z21.Broadcast(protocol.BroadcastFlagXpressNet, protocol.EncodeTrackPowerBC(on), s.xStatusReply())
	s.BroadcastSystemState()
	return true
}
