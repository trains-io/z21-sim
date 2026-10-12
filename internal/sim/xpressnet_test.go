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
