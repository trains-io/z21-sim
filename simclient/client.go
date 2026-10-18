// Package simclient is a typed Go client for the z21-sim control API.
//
// It hides the gRPC and protobuf types of api/simctl/v1, so callers such as
// z21-sim-ctl, integration tests and sidecars depend only on plain Go types.
package simclient

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	simctlv1 "github.com/trains-io/z21-sim/api/simctl/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// DefaultAddr is the default address of the z21-sim control API.
const DefaultAddr = "127.0.0.1:50051"

// Client talks to the control API of one simulator. It is safe for
// concurrent use.
type Client struct {
	conn *grpc.ClientConn
	svc  simctlv1.SimulatorServiceClient
}

// Dial creates a client for the control API at addr ("host:port"). It does
// not connect yet; the first call does.
func Dial(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("simclient: dial %s: %w", addr, err)
	}
	return &Client{conn: conn, svc: simctlv1.NewSimulatorServiceClient(conn)}, nil
}

// Close releases the connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// SetTrackPower switches track power on or off and reports whether the
// state changed. Switching it on also ends an emergency stop and a short
// circuit. Subscribed
// clients are notified of a change.
func (c *Client) SetTrackPower(ctx context.Context, on bool) (changed bool, err error) {
	resp, err := c.svc.SetTrackPower(ctx, &simctlv1.SetTrackPowerRequest{On: on})
	if err != nil {
		return false, fmt.Errorf("simclient: set track power: %w", err)
	}
	return resp.GetChanged(), nil
}

// EmergencyStop stops all locos; the track voltage stays as it is. It
// reports whether no emergency stop was active before. SetTrackPower(ctx,
// true) ends it.
func (c *Client) EmergencyStop(ctx context.Context) (changed bool, err error) {
	resp, err := c.svc.EmergencyStop(ctx, &simctlv1.EmergencyStopRequest{})
	if err != nil {
		return false, fmt.Errorf("simclient: emergency stop: %w", err)
	}
	return resp.GetChanged(), nil
}

// ShortCircuitKind tells where a short circuit happened.
type ShortCircuitKind int

const (
	// ShortCircuitInternal is a short on the Z21's own main or programming
	// track output.
	ShortCircuitInternal ShortCircuitKind = iota
	// ShortCircuitExternal is a short on the output of an external booster.
	ShortCircuitExternal
)

// ShortCircuit reports a short circuit, which switches the track voltage
// off. It reports whether the state changed. SetTrackPower(ctx, true) ends
// it.
func (c *Client) ShortCircuit(ctx context.Context, kind ShortCircuitKind) (changed bool, err error) {
	pk := simctlv1.ShortCircuitKind_SHORT_CIRCUIT_KIND_INTERNAL
	if kind == ShortCircuitExternal {
		pk = simctlv1.ShortCircuitKind_SHORT_CIRCUIT_KIND_EXTERNAL
	}
	resp, err := c.svc.ShortCircuit(ctx, &simctlv1.ShortCircuitRequest{Kind: pk})
	if err != nil {
		return false, fmt.Errorf("simclient: short circuit: %w", err)
	}
	return resp.GetChanged(), nil
}

// CANDetector is a CAN occupancy detector to add (spec §10.1).
type CANDetector struct {
	NetID      uint16 // CAN network ID, not 0
	Name       string // at most 15 bytes
	ModuleAddr uint16 // 0 means 1
	PortCount  uint16 // 0 means 1; at most 256
}

// AddCANDetector registers a detector, as if it had just been plugged in,
// and sends its occupancy reports (all ports free, with voltage) to the
// CAN detector subscribers. It returns how many were notified. Adding a
// detector with the NetID of an existing device replaces that device.
func (c *Client) AddCANDetector(ctx context.Context, d CANDetector) (recipients int, err error) {
	resp, err := c.svc.NotifyCANDeviceDetected(ctx, &simctlv1.NotifyCANDeviceDetectedRequest{
		NetId:      uint32(d.NetID),
		Kind:       simctlv1.CANDeviceKind_CAN_DEVICE_KIND_DETECTOR,
		Name:       d.Name,
		ModuleAddr: uint32(d.ModuleAddr),
		PortCount:  uint32(d.PortCount),
	})
	if err != nil {
		return 0, fmt.Errorf("simclient: add CAN detector: %w", err)
	}
	return int(resp.GetRecipientCount()), nil
}

// CANBooster is a CAN booster to add (spec §10.2).
type CANBooster struct {
	NetID      uint16 // CAN network ID, not 0
	Name       string // at most 15 bytes
	OutputPort uint16 // 0 means 1
}

// AddCANBooster registers a booster, as if it had just been plugged in, and
// sends its system state to the CAN booster subscribers. It returns how
// many were notified. Adding a booster with the NetID of an existing device
// replaces that device.
func (c *Client) AddCANBooster(ctx context.Context, b CANBooster) (recipients int, err error) {
	resp, err := c.svc.NotifyCANDeviceDetected(ctx, &simctlv1.NotifyCANDeviceDetectedRequest{
		NetId:      uint32(b.NetID),
		Kind:       simctlv1.CANDeviceKind_CAN_DEVICE_KIND_BOOSTER,
		Name:       b.Name,
		OutputPort: uint32(b.OutputPort),
	})
	if err != nil {
		return 0, fmt.Errorf("simclient: add CAN booster: %w", err)
	}
	return int(resp.GetRecipientCount()), nil
}

// BroadcastSystemState sends the current system state to the system-state
// subscribers and returns how many were notified.
func (c *Client) BroadcastSystemState(ctx context.Context) (recipients int, err error) {
	resp, err := c.svc.BroadcastSystemState(ctx, &simctlv1.BroadcastSystemStateRequest{})
	if err != nil {
		return 0, fmt.Errorf("simclient: broadcast system state: %w", err)
	}
	return int(resp.GetRecipientCount()), nil
}

// SystemValues are the measured system-state values to change. Nil fields
// are left unchanged.
type SystemValues struct {
	MainCurrent         *int16  // mA
	ProgCurrent         *int16  // mA
	FilteredMainCurrent *int16  // mA
	Temperature         *int16  // °C
	SupplyVoltage       *uint16 // mV
	VCCVoltage          *uint16 // mV
}

// SetSystemState changes the measured system-state values. It reports
// whether the reported state changed and how many system-state subscribers
// were notified. While track power is off, the currents and the VCC voltage
// are reported as zero; values set meanwhile are reported once power is on.
func (c *Client) SetSystemState(ctx context.Context, v SystemValues) (changed bool, recipients int, err error) {
	resp, err := c.svc.SetSystemState(ctx, &simctlv1.SetSystemStateRequest{
		MainCurrentMa:         widen[int16, int32](v.MainCurrent),
		ProgCurrentMa:         widen[int16, int32](v.ProgCurrent),
		FilteredMainCurrentMa: widen[int16, int32](v.FilteredMainCurrent),
		TemperatureC:          widen[int16, int32](v.Temperature),
		SupplyVoltageMv:       widen[uint16, uint32](v.SupplyVoltage),
		VccVoltageMv:          widen[uint16, uint32](v.VCCVoltage),
	})
	if err != nil {
		return false, 0, fmt.Errorf("simclient: set system state: %w", err)
	}
	return resp.GetChanged(), int(resp.GetRecipientCount()), nil
}

func widen[From int16 | uint8 | uint16, To int32 | uint32](v *From) *To {
	if v == nil {
		return nil
	}
	w := To(*v)
	return &w
}

// SpeedSteps is the speed step mode of a loco: 14, 28 or 128.
type SpeedSteps int

const (
	SpeedSteps14  SpeedSteps = 14
	SpeedSteps28  SpeedSteps = 28
	SpeedSteps128 SpeedSteps = 128
)

// LocoChange is a change to a loco. Zero and nil fields and functions missing
// from Functions stay unchanged.
type LocoChange struct {
	// SpeedSteps is the new speed step mode. Without Speed, the loco keeps
	// its speed step, which must exist in the new mode, or its emergency
	// stop.
	SpeedSteps SpeedSteps
	// Speed is the speed step: 0 stops, at most 14, 28 or 126 depending on
	// the speed step mode. It ends an emergency stop.
	Speed     *uint8
	Forward   *bool
	Functions map[uint8]bool // F0–F31, on or off
}

// Loco is the state of a loco, as reported in LAN_X_LOCO_INFO.
type Loco struct {
	Address       uint16
	SpeedSteps    SpeedSteps
	Speed         uint8 // speed step, 0 when stopped or in an emergency stop
	EmergencyStop bool
	Forward       bool
	Functions     uint32 // functions that are on, F0 in bit 0
}

// Function reports whether function fn (F0–F31) is on.
func (l Loco) Function(fn uint8) bool {
	return fn < 32 && l.Functions&(1<<fn) != 0
}

// SetLoco changes the loco with addr (1–9999), as a throttle outside the
// Z21 LAN would. It returns the loco after the change, whether it changed
// and how many clients subscribed to the loco got LAN_X_LOCO_INFO.
func (c *Client) SetLoco(ctx context.Context, addr uint16, ch LocoChange) (loco Loco, changed bool, recipients int, err error) {
	req := &simctlv1.SetLocoRequest{
		Address:   uint32(addr),
		Speed:     widen[uint8, uint32](ch.Speed),
		Forward:   ch.Forward,
		Functions: make(map[uint32]bool, len(ch.Functions)),
	}
	if ch.SpeedSteps != 0 {
		steps, ok := speedStepsToProto[ch.SpeedSteps]
		if !ok {
			return Loco{}, false, 0, fmt.Errorf("simclient: set loco: unknown speed steps %d", ch.SpeedSteps)
		}
		req.SpeedSteps = &steps
	}
	for fn, on := range ch.Functions {
		req.Functions[uint32(fn)] = on
	}
	resp, err := c.svc.SetLoco(ctx, req)
	if err != nil {
		return Loco{}, false, 0, fmt.Errorf("simclient: set loco: %w", err)
	}
	pl := resp.GetLoco()
	loco = Loco{
		Address:       uint16(pl.GetAddress()),
		Speed:         uint8(pl.GetSpeed()),
		EmergencyStop: pl.GetEmergencyStop(),
		Forward:       pl.GetForward(),
		Functions:     pl.GetFunctions(),
	}
	for steps, ps := range speedStepsToProto {
		if ps == pl.GetSpeedSteps() {
			loco.SpeedSteps = steps
		}
	}
	return loco, resp.GetChanged(), int(resp.GetRecipientCount()), nil
}

var speedStepsToProto = map[SpeedSteps]simctlv1.LocoSpeedSteps{
	SpeedSteps14:  simctlv1.LocoSpeedSteps_LOCO_SPEED_STEPS_14,
	SpeedSteps28:  simctlv1.LocoSpeedSteps_LOCO_SPEED_STEPS_28,
	SpeedSteps128: simctlv1.LocoSpeedSteps_LOCO_SPEED_STEPS_128,
}

// ClientInfo describes the session of a Z21 LAN client.
type ClientInfo struct {
	Addr           netip.AddrPort // client UDP address
	LocalAddr      netip.AddrPort // simulator socket the client last sent to
	BroadcastFlags uint32
	Locos          []uint16 // subscribed loco addresses, oldest first
	LastSeen       time.Time
}

// Clients returns the Z21 LAN clients with an active session, ordered by
// address.
func (c *Client) Clients(ctx context.Context) ([]ClientInfo, error) {
	resp, err := c.svc.ListClients(ctx, &simctlv1.ListClientsRequest{})
	if err != nil {
		return nil, fmt.Errorf("simclient: list clients: %w", err)
	}
	clients := make([]ClientInfo, 0, len(resp.GetClients()))
	for _, pc := range resp.GetClients() {
		info, err := clientInfoFromProto(pc)
		if err != nil {
			return nil, fmt.Errorf("simclient: list clients: %w", err)
		}
		clients = append(clients, info)
	}
	return clients, nil
}

func clientInfoFromProto(pc *simctlv1.Client) (ClientInfo, error) {
	addr, err := netip.ParseAddrPort(pc.GetAddr())
	if err != nil {
		return ClientInfo{}, fmt.Errorf("client address: %w", err)
	}
	localAddr, err := netip.ParseAddrPort(pc.GetLocalAddr())
	if err != nil {
		return ClientInfo{}, fmt.Errorf("local address: %w", err)
	}
	locos := make([]uint16, len(pc.GetLocos()))
	for i, l := range pc.GetLocos() {
		locos[i] = uint16(l)
	}
	return ClientInfo{
		Addr:           addr,
		LocalAddr:      localAddr,
		BroadcastFlags: pc.GetBroadcastFlags(),
		Locos:          locos,
		LastSeen:       pc.GetLastSeen().AsTime(),
	}, nil
}
