package sim

import (
	"github.com/trains-io/z21.go/protocol"
)

const (
	centralStateTrackVoltageOff byte = 0x02
	defaultSupplyVoltageMV           = 10000
	defaultVCCVoltageMV              = 1000
	defaultMainCurrentMA             = 100
	defaultTemperatureC              = 42
	defaultCapabilities         byte = 0x31 // DCC + loco + accessory commands
)

func (s *Server) systemStateReply() protocol.Message {
	return protocol.EncodeSystemState(protocol.SystemState{
		MainCurrent:         defaultMainCurrentMA,
		FilteredMainCurrent: defaultMainCurrentMA - 20,
		Temperature:         defaultTemperatureC,
		SupplyVoltage:       defaultSupplyVoltageMV,
		VCCVoltage:          defaultVCCVoltageMV,
		CentralState:        s.centralState(),
		Capabilities:        defaultCapabilities,
	})
}
