package sim

import "github.com/trains-io/z21.go/protocol"

// locoInfoSpeedSteps128 is 128 speed steps in LAN_X_LOCO_INFO (spec §4.4);
// LAN_X_SET_LOCO_DRIVE uses protocol.LocoSpeedSteps128 instead.
const locoInfoSpeedSteps128 byte = 0x04

// defaultLoco is a loco nobody has driven yet: DCC with 128 speed steps,
// forward, stopped, all functions off, as in ZIMO's simulator.
func defaultLoco(addr uint16) protocol.LocoInfo {
	return protocol.LocoInfo{Address: addr, SpeedSteps: locoInfoSpeedSteps128, Forward: true}
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
		info.SpeedSteps = locoInfoSpeedSteps(d.SpeedSteps)
		info.Forward = d.Forward
		info.Speed = d.Speed
	})
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

// locoInfoSpeedSteps converts speed steps of LAN_X_SET_LOCO_DRIVE to those of
// LAN_X_LOCO_INFO.
func locoInfoSpeedSteps(drive byte) byte {
	if drive == protocol.LocoSpeedSteps128 {
		return locoInfoSpeedSteps128
	}
	return drive
}
