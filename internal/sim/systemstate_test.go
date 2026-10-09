package sim

import (
	"testing"

	"github.com/trains-io/z21.go/protocol"
)

func TestSystemStateReplyTrackPowerOn(t *testing.T) {
	s := &Server{trackPower: true}
	state, err := protocol.ParseSystemState(s.systemStateReply().Data)
	if err != nil {
		t.Fatalf("ParseSystemState: %v", err)
	}
	if got := protocol.FormatXStatusFlags(state.CentralState); got != "ok" {
		t.Fatalf("CentralState = %q, want ok", got)
	}
}

func TestSystemStateReplyTrackPowerOff(t *testing.T) {
	s := &Server{trackPower: false}
	state, err := protocol.ParseSystemState(s.systemStateReply().Data)
	if err != nil {
		t.Fatalf("ParseSystemState: %v", err)
	}
	if got := protocol.FormatXStatusFlags(state.CentralState); got != "track voltage off" {
		t.Fatalf("CentralState = %q, want track voltage off", got)
	}
}
