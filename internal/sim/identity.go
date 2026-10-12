package sim

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/trains-io/z21.go/protocol"
)

// Identity is what the simulated command station reports about itself. It is
// fixed when the simulator starts.
type Identity struct {
	Serial uint32 // LAN_GET_SERIAL_NUMBER
	HwType uint32 // protocol.HwType*, LAN_GET_HWINFO
	// Firmware is the BCD firmware version, for example 0x0143 for 1.43,
	// reported by LAN_GET_HWINFO and LAN_X_GET_FIRMWARE_VERSION.
	Firmware     uint32
	Code         byte // protocol.Code*, LAN_GET_CODE
	Capabilities byte // protocol.Capability*, in LAN_SYSTEMSTATE_DATACHANGED
}

const (
	defaultFirmware     uint32 = 0x0120 // 1.20
	defaultCapabilities        = protocol.CapabilityDCC | protocol.CapabilityLocoCmds | protocol.CapabilityAccessoryCmds
)

// DefaultIdentity is a black Z21 (2013) with firmware 1.20, no lock and DCC
// loco and accessory commands. Its serial number is 999 followed by six
// digits from the current time, which tells simulators from real hardware.
func DefaultIdentity() Identity {
	return Identity{
		Serial:       makeSimulatorSerialNumber(time.Now()),
		HwType:       protocol.HwTypeZ21New,
		Firmware:     defaultFirmware,
		Code:         protocol.CodeNoLock,
		Capabilities: defaultCapabilities,
	}
}

// hwTypeNames are the command station hardware types ParseHwType accepts by
// name.
var hwTypeNames = map[string]uint32{
	"z21-old":   protocol.HwTypeZ21Old,
	"z21":       protocol.HwTypeZ21New,
	"smartrail": protocol.HwTypeSmartRail,
	"z21-small": protocol.HwTypeZ21Small,
	"z21-start": protocol.HwTypeZ21Start,
	"z21-xl":    protocol.HwTypeZ21XL,
}

// HwTypeNames returns the names ParseHwType accepts, sorted.
func HwTypeNames() []string {
	names := make([]string, 0, len(hwTypeNames))
	for name := range hwTypeNames {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// ParseHwType parses a hardware type name (see HwTypeNames) or a number in
// decimal or 0x hex, for example 0x0211.
func ParseHwType(s string) (uint32, error) {
	if hw, ok := hwTypeNames[s]; ok {
		return hw, nil
	}
	n, err := strconv.ParseUint(s, 0, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid hardware type %q: want one of %s, or a number", s, strings.Join(HwTypeNames(), ", "))
	}
	return uint32(n), nil
}

// ParseFirmwareVersion parses "major.minor" with a two-digit minor, for
// example "1.43", into its BCD form (0x0143).
func ParseFirmwareVersion(s string) (uint32, error) {
	majorStr, minorStr, ok := strings.Cut(s, ".")
	major, errMajor := strconv.ParseUint(majorStr, 10, 8)
	minor, errMinor := strconv.ParseUint(minorStr, 10, 8)
	if !ok || errMajor != nil || errMinor != nil || len(minorStr) != 2 || major > 99 {
		return 0, fmt.Errorf("invalid firmware version %q: want major.minor such as 1.43", s)
	}
	return uint32(toBCD(byte(major)))<<8 | uint32(toBCD(byte(minor))), nil
}

func toBCD(n byte) byte {
	return n/10<<4 | n%10
}

var codeNames = map[string]byte{
	"none":           protocol.CodeNoLock,
	"start-locked":   protocol.CodeZ21StartLocked,
	"start-unlocked": protocol.CodeZ21StartUnlocked,
}

// ParseCode parses a lock code: none, start-locked or start-unlocked.
func ParseCode(s string) (byte, error) {
	if code, ok := codeNames[s]; ok {
		return code, nil
	}
	return 0, fmt.Errorf("invalid code %q: want none, start-locked or start-unlocked", s)
}
