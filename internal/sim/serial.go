package sim

import (
	"time"

	"github.com/trains-io/z21.go/protocol"
)

// Decimal serials starting with 999 identify z21-sim instances (vs physical Z21 hardware).
const simSerialPrefix uint32 = 999

func makeSimulatorSerialNumber(now time.Time) uint32 {
	epochSeconds := uint32(now.Unix())
	return simSerialPrefix*1_000_000 + (epochSeconds % 1_000_000)
}

func (s *Server) serialReply() protocol.Message {
	return protocol.EncodeSerialNumber(s.identity.Serial)
}

func (s *Server) codeReply() protocol.Message {
	return protocol.EncodeCode(s.identity.Code)
}
