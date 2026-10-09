package sim

import (
	"testing"

	"github.com/trains-io/z21.go/protocol"
)

func TestXFirmwareReply(t *testing.T) {
	s := &Server{}
	reply := s.xFirmwareReply()

	fw, err := protocol.ParseXFirmware(reply.Data)
	if err != nil {
		t.Fatalf("ParseXFirmware: %v", err)
	}
	if got := protocol.FormatXFirmwareVersion(fw); got != "1.20" {
		t.Fatalf("FormatXFirmwareVersion() = %q, want 1.20", got)
	}
}

func TestXStatusReplyTrackPowerOn(t *testing.T) {
	s := &Server{trackPower: true}
	reply := s.xStatusReply()

	status, err := protocol.ParseXStatus(reply.Data)
	if err != nil {
		t.Fatalf("ParseXStatus: %v", err)
	}
	if got := protocol.FormatXStatusFlags(status.CentralState); got != "ok" {
		t.Fatalf("CentralState = %q, want ok", got)
	}
}
