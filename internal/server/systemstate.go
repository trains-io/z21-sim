package server

import (
	"encoding/binary"

	"github.com/trains-io/z21.go/protocol"
)

const (
	centralStateTrackVoltageOff byte = 0x02
	defaultSupplyVoltageMV           = 10000
	defaultVCCVoltageMV              = 1000
	defaultMainCurrentMA             = 100
	defaultTemperatureC              = 42
	defaultCapabilities       byte   = 0x31 // DCC + loco + accessory commands
)

func (s *Server) systemStateReply() protocol.Message {
	return protocol.Message{
		Header: protocol.HeaderLANSystemStateDataChanged,
		Data:   s.encodeSystemState(),
	}
}

func (s *Server) encodeSystemState() []byte {
	data := make([]byte, 16)

	binary.LittleEndian.PutUint16(data[0:2], defaultMainCurrentMA)
	binary.LittleEndian.PutUint16(data[4:6], defaultMainCurrentMA-20)
	binary.LittleEndian.PutUint16(data[6:8], defaultTemperatureC)
	binary.LittleEndian.PutUint16(data[8:10], defaultSupplyVoltageMV)
	binary.LittleEndian.PutUint16(data[10:12], defaultVCCVoltageMV)

	if !s.trackPower {
		data[12] = centralStateTrackVoltageOff
	}

	data[15] = defaultCapabilities
	return data
}
