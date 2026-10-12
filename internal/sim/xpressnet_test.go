package sim

import (
	"testing"

	"github.com/trains-io/z21.go/protocol"
)

func TestXFirmwareReply(t *testing.T) {
	tests := []struct {
		firmware uint32
		want     string
	}{
		{firmware: defaultFirmware, want: "1.20"},
		{firmware: 0x0143, want: "1.43"},
	}
	for _, tt := range tests {
		s := &Server{identity: Identity{Firmware: tt.firmware}}
		fw, err := protocol.ParseXFirmware(s.xFirmwareReply().Data)
		if err != nil {
			t.Fatalf("ParseXFirmware: %v", err)
		}
		if got := protocol.FormatXFirmwareVersion(fw); got != tt.want {
			t.Fatalf("FormatXFirmwareVersion() = %q, want %s", got, tt.want)
		}
	}
}
