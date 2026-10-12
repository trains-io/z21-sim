package sim_test

import (
	"context"
	"testing"
	"time"

	"github.com/trains-io/z21-sim/internal/sim"
	"github.com/trains-io/z21.go/client"
	"github.com/trains-io/z21.go/protocol"
)

func TestDefaultIdentity(t *testing.T) {
	id := sim.DefaultIdentity()
	if id.Serial/1_000_000 != 999 || id.HwType != protocol.HwTypeZ21New || id.Firmware != 0x0120 || id.Code != protocol.CodeNoLock {
		t.Fatalf("DefaultIdentity() = %+v, want 999-prefix serial, black Z21 (2013), firmware 1.20, no lock", id)
	}
	if id.Capabilities != sim.NewState().System().Capabilities {
		t.Fatalf("capabilities = %#x, want those of NewState (%#x)", id.Capabilities, sim.NewState().System().Capabilities)
	}
}

func TestParseHwType(t *testing.T) {
	tests := []struct {
		in   string
		want uint32
	}{
		{in: "z21", want: protocol.HwTypeZ21New},
		{in: "z21-old", want: protocol.HwTypeZ21Old},
		{in: "z21-start", want: protocol.HwTypeZ21Start},
		{in: "z21-xl", want: protocol.HwTypeZ21XL},
		{in: "0x0205", want: protocol.HwTypeSingleBooster},
		{in: "513", want: protocol.HwTypeZ21New},
	}
	for _, tt := range tests {
		if got, err := sim.ParseHwType(tt.in); err != nil || got != tt.want {
			t.Errorf("ParseHwType(%q) = %#x, %v; want %#x", tt.in, got, err, tt.want)
		}
	}
	for _, in := range []string{"", "Z21", "z21-big", "0x1_0000_0000"} {
		if _, err := sim.ParseHwType(in); err == nil {
			t.Errorf("ParseHwType(%q) succeeded, want an error", in)
		}
	}
}

func TestParseFirmwareVersion(t *testing.T) {
	tests := []struct {
		in   string
		want uint32
	}{
		{in: "1.20", want: 0x0120},
		{in: "1.43", want: 0x0143},
		{in: "10.05", want: 0x1005},
		{in: "0.00", want: 0x0000},
		{in: "99.99", want: 0x9999},
	}
	for _, tt := range tests {
		got, err := sim.ParseFirmwareVersion(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("ParseFirmwareVersion(%q) = %#x, %v; want %#x", tt.in, got, err, tt.want)
		}
		if formatted := protocol.FormatFirmwareVersion(got); formatted != tt.in {
			t.Errorf("FormatFirmwareVersion(ParseFirmwareVersion(%q)) = %q", tt.in, formatted)
		}
	}
	for _, in := range []string{"", "1", "1.5", "1.234", "100.00", "a.bc", "1.4x", "-1.00", "1.-5"} {
		if _, err := sim.ParseFirmwareVersion(in); err == nil {
			t.Errorf("ParseFirmwareVersion(%q) succeeded, want an error", in)
		}
	}
}

func TestParseCode(t *testing.T) {
	tests := []struct {
		in   string
		want byte
	}{
		{in: "none", want: protocol.CodeNoLock},
		{in: "start-locked", want: protocol.CodeZ21StartLocked},
		{in: "start-unlocked", want: protocol.CodeZ21StartUnlocked},
	}
	for _, tt := range tests {
		if got, err := sim.ParseCode(tt.in); err != nil || got != tt.want {
			t.Errorf("ParseCode(%q) = %#x, %v; want %#x", tt.in, got, err, tt.want)
		}
	}
	if _, err := sim.ParseCode("locked"); err == nil {
		t.Error(`ParseCode("locked") succeeded, want an error`)
	}
}

func TestWithIdentity(t *testing.T) {
	id := sim.Identity{
		Serial:       123456,
		HwType:       protocol.HwTypeZ21XL,
		Firmware:     0x0143,
		Code:         protocol.CodeZ21StartLocked,
		Capabilities: protocol.CapabilityDCC | protocol.CapabilityMM | protocol.CapabilityRailCom,
	}
	_, addr := startTestServer(t, sim.WithIdentity(id))

	c, err := client.Dial(addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	call := func(req protocol.Message) []protocol.Message {
		t.Helper()
		msgs, err := c.Call(ctx, req)
		if err != nil {
			t.Fatalf("Call %s: %v", protocol.MessageName(req), err)
		}
		return msgs
	}

	if serial, ok := protocol.SerialFromMessages(call(protocol.GetSerialNumber())); !ok || serial != "123456" {
		t.Errorf("serial = %q (ok %v), want 123456", serial, ok)
	}
	if hw, err := protocol.HWInfoFromMessages(call(protocol.GetHWInfo())); err != nil || hw.HwType != protocol.HwTypeZ21XL || hw.FirmwareVersion != 0x0143 {
		t.Errorf("HW info = %+v, err=%v; want Z21 XL with firmware 1.43", hw, err)
	}
	if code, err := protocol.CodeFromMessages(call(protocol.GetCode())); err != nil || code != protocol.CodeZ21StartLocked {
		t.Errorf("code = %#x, err=%v; want z21 start locked", code, err)
	}
	if fw, err := protocol.XFirmwareFromMessages(call(protocol.GetXFirmware())); err != nil || protocol.FormatXFirmwareVersion(fw) != "1.43" {
		t.Errorf("X firmware = %+v, err=%v; want 1.43", fw, err)
	}
	if state, err := protocol.SystemStateFromMessages(call(protocol.SystemStateGetData())); err != nil || state.Capabilities != id.Capabilities {
		t.Errorf("system state = %+v, err=%v; want capabilities %#x", state, err, id.Capabilities)
	}
}
