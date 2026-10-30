package sim

import (
	"errors"
	"fmt"

	"github.com/trains-io/z21.go/protocol"
)

// MaxAccessoryAddress is the highest turnout and extended accessory address
// of a Z21. Addresses are FAdr as on the wire: 0 is the first turnout.
const MaxAccessoryAddress = 2047

// ErrInvalidAccessory is returned for a turnout or extended accessory command
// that cannot be applied.
var ErrInvalidAccessory = errors.New("z21-sim: invalid accessory command")

// Turnout returns the turnout with addr, as reported by LAN_X_TURNOUT_INFO. A
// turnout nobody has switched yet is protocol.TurnoutPositionNotSwitched.
func (st *State) Turnout(addr uint16) protocol.TurnoutInfo {
	st.mu.Lock()
	defer st.mu.Unlock()
	return protocol.TurnoutInfo{Address: addr, Position: st.turnouts[addr]}
}

// SwitchTurnout applies LAN_X_SET_TURNOUT (spec §5.2) to addr
// (0–MaxAccessoryAddress): activating an output sets the position to output 1
// or 2, deactivating it keeps the position. The queue bit is ignored, the
// simulator switches at once. X-Bus subscribers get LAN_X_TURNOUT_INFO after
// every command, also when the position stays the same, as a real Z21
// confirms each command. It returns the turnout after the command and the
// broadcast, or an error wrapping ErrInvalidAccessory.
func (st *State) SwitchTurnout(addr uint16, sw protocol.TurnoutSwitch) (protocol.TurnoutInfo, []Change, error) {
	if err := checkAccessoryAddress(addr); err != nil {
		return protocol.TurnoutInfo{}, nil, err
	}

	st.mu.Lock()
	if sw.Activate {
		pos := protocol.TurnoutPositionOutput1
		if sw.Output2 {
			pos = protocol.TurnoutPositionOutput2
		}
		st.turnouts[addr] = pos
	}
	info := protocol.TurnoutInfo{Address: addr, Position: st.turnouts[addr]}
	st.mu.Unlock()

	return info, []Change{xBusAccessoryChange(protocol.EncodeTurnoutInfo(info))}, nil
}

// ExtAccessory returns the extended accessory with addr, as reported by
// LAN_X_EXT_ACCESSORY_INFO. An accessory nobody has set yet has status
// protocol.ExtAccessoryStatusUnknown.
func (st *State) ExtAccessory(addr uint16) protocol.ExtAccessoryInfo {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.extAccessoryLocked(addr)
}

// SetExtAccessory applies LAN_X_SET_EXT_ACCESSORY (spec §5.4): addr
// (0–MaxAccessoryAddress) gets value. X-Bus subscribers get
// LAN_X_EXT_ACCESSORY_INFO after every command, also when the value stays the
// same. It returns the accessory after the command and the broadcast, or an
// error wrapping ErrInvalidAccessory.
func (st *State) SetExtAccessory(addr uint16, value byte) (protocol.ExtAccessoryInfo, []Change, error) {
	if err := checkAccessoryAddress(addr); err != nil {
		return protocol.ExtAccessoryInfo{}, nil, err
	}

	st.mu.Lock()
	st.extAccessories[addr] = value
	info := st.extAccessoryLocked(addr)
	st.mu.Unlock()

	return info, []Change{xBusAccessoryChange(protocol.EncodeExtAccessoryInfo(info))}, nil
}

// extAccessoryLocked returns the state of addr. st.mu must be held.
func (st *State) extAccessoryLocked(addr uint16) protocol.ExtAccessoryInfo {
	value, ok := st.extAccessories[addr]
	if !ok {
		return protocol.ExtAccessoryInfo{Address: addr, Status: protocol.ExtAccessoryStatusUnknown}
	}
	return protocol.ExtAccessoryInfo{Address: addr, Value: value, Status: protocol.ExtAccessoryStatusValid}
}

func checkAccessoryAddress(addr uint16) error {
	if addr > MaxAccessoryAddress {
		return fmt.Errorf("%w: address %d out of range [0, %d]", ErrInvalidAccessory, addr, MaxAccessoryAddress)
	}
	return nil
}

// xBusAccessoryChange sends msg to the X-Bus subscribers, which get turnout
// and accessory info (spec §2.16).
func xBusAccessoryChange(msg protocol.Message) Change {
	return Change{Flag: protocol.BroadcastFlagXpressNet, Messages: []protocol.Message{msg}}
}
