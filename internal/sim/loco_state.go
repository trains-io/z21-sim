package sim

import (
	"errors"
	"fmt"

	"github.com/trains-io/z21.go/protocol"
)

// MaxLocoAddress is the highest loco address of a Z21.
const MaxLocoAddress = 9999

// ErrInvalidLoco is returned by SetLoco for a change it cannot apply.
var ErrInvalidLoco = errors.New("z21-sim: invalid loco change")

// LocoChange is a change to a loco made by a throttle outside the Z21 LAN.
// Nil fields and functions missing from Functions stay unchanged.
type LocoChange struct {
	SpeedSteps *byte          // protocol.LocoSpeedSteps*
	Speed      *uint8         // speed step, 0 stops; at most protocol.MaxLocoSpeedStep
	Forward    *bool          // direction
	Functions  map[uint8]bool // F0–F31, on or off
}

// defaultLoco is a loco nobody has driven yet: DCC with 128 speed steps,
// forward, stopped, all functions off, as in ZIMO's simulator.
func defaultLoco(addr uint16) protocol.LocoInfo {
	return protocol.LocoInfo{Address: addr, SpeedSteps: protocol.LocoInfoSpeedSteps128, Forward: true}
}

// Loco returns the state of the loco with addr, as reported by
// LAN_X_LOCO_INFO.
func (st *State) Loco(addr uint16) protocol.LocoInfo {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.locoLocked(addr)
}

// DriveLoco sets the speed steps, direction and speed of d.Address, as
// LAN_X_SET_LOCO_DRIVE does. It returns the new state and whether it changed:
// the loco's subscribers learn about changes only.
func (st *State) DriveLoco(d protocol.LocoDrive) (protocol.LocoInfo, bool) {
	return st.updateLoco(d.Address, func(info *protocol.LocoInfo) {
		info.SpeedSteps = protocol.LocoInfoSpeedSteps(d.SpeedSteps)
		info.Forward = d.Forward
		info.Speed = d.Speed
	})
}

// SetLocoFunction switches function fn (F0–F31) of addr on, off or over, as
// LAN_X_SET_LOCO_FUNCTION does. Higher functions are not reported by
// LAN_X_LOCO_INFO and are ignored. It returns the new state and whether it
// changed.
func (st *State) SetLocoFunction(addr uint16, fn uint8, action protocol.LocoFunctionAction) (protocol.LocoInfo, bool) {
	return st.updateLoco(addr, func(info *protocol.LocoInfo) {
		if fn > 31 {
			return
		}
		bit := uint32(1) << fn
		f := LocoFunctions(*info)
		switch action {
		case protocol.LocoFunctionOn:
			f |= bit
		case protocol.LocoFunctionOff:
			f &^= bit
		case protocol.LocoFunctionToggle:
			f ^= bit
		}
		setLocoFunctions(info, f)
	})
}

// SetLocoFunctionGroup sets the functions of group for addr, as
// LAN_X_SET_LOCO_FUNCTION_GROUP does (spec §4.3.2). Of the groups above F28
// only F29–F31 are applied, the functions LAN_X_LOCO_INFO reports. It returns
// the new state and whether it changed.
func (st *State) SetLocoFunctionGroup(addr uint16, group protocol.LocoFunctionGroup, functions byte) (protocol.LocoInfo, bool) {
	mask, bits := locoFunctionGroupBits(group, functions)
	return st.updateLoco(addr, func(info *protocol.LocoInfo) {
		setLocoFunctions(info, LocoFunctions(*info)&^mask|bits)
	})
}

// EStopLoco stops addr at once, keeping its direction, as LAN_X_SET_LOCO_E_STOP
// does. It returns the new state and whether it changed.
func (st *State) EStopLoco(addr uint16) (protocol.LocoInfo, bool) {
	return st.updateLoco(addr, func(info *protocol.LocoInfo) {
		info.Speed = protocol.LocoSpeedEStop
	})
}

// SetLoco applies c to addr (1–MaxLocoAddress). A new speed step mode without
// a new speed keeps the speed step, or the emergency stop. It returns the new
// state and whether it changed, or an error wrapping ErrInvalidLoco, leaving
// the loco unchanged.
func (st *State) SetLoco(addr uint16, c LocoChange) (protocol.LocoInfo, bool, error) {
	if addr == 0 || addr > MaxLocoAddress {
		return protocol.LocoInfo{}, false, fmt.Errorf("%w: address %d out of range [1, %d]", ErrInvalidLoco, addr, MaxLocoAddress)
	}
	if c.SpeedSteps != nil && protocol.MaxLocoSpeedStep(*c.SpeedSteps) == 0 {
		return protocol.LocoInfo{}, false, fmt.Errorf("%w: unknown speed steps %#02x", ErrInvalidLoco, *c.SpeedSteps)
	}
	for fn := range c.Functions {
		if fn > 31 {
			return protocol.LocoInfo{}, false, fmt.Errorf("%w: function F%d out of range [F0, F31]", ErrInvalidLoco, fn)
		}
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	before := st.locoLocked(addr)
	after := before

	speedSteps := protocol.LocoDriveSpeedSteps(before.SpeedSteps)
	step, eStop := protocol.DecodeLocoSpeed(speedSteps, before.Speed)
	if c.SpeedSteps != nil {
		speedSteps = *c.SpeedSteps
		after.SpeedSteps = protocol.LocoInfoSpeedSteps(speedSteps)
	}
	if c.Speed != nil {
		step, eStop = *c.Speed, false
	}
	if eStop {
		after.Speed = protocol.LocoSpeedEStop
	} else {
		speed, err := protocol.EncodeLocoSpeed(speedSteps, step)
		if err != nil {
			return protocol.LocoInfo{}, false, fmt.Errorf("%w: %v", ErrInvalidLoco, err)
		}
		after.Speed = speed
	}

	if c.Forward != nil {
		after.Forward = *c.Forward
	}

	f := LocoFunctions(after)
	for fn, on := range c.Functions {
		if on {
			f |= 1 << fn
		} else {
			f &^= 1 << fn
		}
	}
	setLocoFunctions(&after, f)

	st.locos[addr] = after
	return after, after != before, nil
}

// PurgeLoco forgets addr, as LAN_X_PURGE_LOCO does: it is a default loco
// again.
func (st *State) PurgeLoco(addr uint16) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.locos, addr)
}

func (st *State) updateLoco(addr uint16, update func(*protocol.LocoInfo)) (protocol.LocoInfo, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	before := st.locoLocked(addr)
	after := before
	update(&after)
	st.locos[addr] = after
	return after, after != before
}

// locoLocked returns the state of addr. st.mu must be held.
func (st *State) locoLocked(addr uint16) protocol.LocoInfo {
	if info, ok := st.locos[addr]; ok {
		return info
	}
	return defaultLoco(addr)
}

// LocoFunctions returns the functions of info as a bit mask, F0 in bit 0.
func LocoFunctions(info protocol.LocoInfo) uint32 {
	f := uint32(info.FunctionsF1F4&0x0F)<<1 |
		uint32(info.FunctionsF5F12)<<5 |
		uint32(info.FunctionsF13F20)<<13 |
		uint32(info.FunctionsF21F28)<<21 |
		uint32(info.FunctionsF29F31&0x07)<<29
	if info.Headlight {
		f |= 1
	}
	return f
}

// setLocoFunctions stores the bit mask f, F0 in bit 0, in info.
func setLocoFunctions(info *protocol.LocoInfo, f uint32) {
	info.Headlight = f&1 != 0
	info.FunctionsF1F4 = byte(f>>1) & 0x0F
	info.FunctionsF5F12 = byte(f >> 5)
	info.FunctionsF13F20 = byte(f >> 13)
	info.FunctionsF21F28 = byte(f >> 21)
	info.FunctionsF29F31 = byte(f>>29) & 0x07
}

// locoFunctionGroupBits returns the functions group covers and those of them
// that functions switches on, as bit masks with F0 in bit 0.
func locoFunctionGroupBits(group protocol.LocoFunctionGroup, functions byte) (mask, bits uint32) {
	f := uint32(functions)
	switch group {
	case protocol.LocoFunctionGroupF0F4: // F0 in bit 4, F1–F4 in bits 0–3
		return 0x1F, (f&0x0F)<<1 | (f>>4)&1
	case protocol.LocoFunctionGroupF5F8:
		return 0x0F << 5, (f & 0x0F) << 5
	case protocol.LocoFunctionGroupF9F12:
		return 0x0F << 9, (f & 0x0F) << 9
	case protocol.LocoFunctionGroupF13F20:
		return 0xFF << 13, f << 13
	case protocol.LocoFunctionGroupF21F28:
		return 0xFF << 21, f << 21
	case protocol.LocoFunctionGroupF29F36:
		return 0x07 << 29, (f & 0x07) << 29
	default:
		return 0, 0
	}
}
