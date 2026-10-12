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
// state changed. Switching it on also ends an emergency stop. Subscribed
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

func widen[From int16 | uint16, To int32 | uint32](v *From) *To {
	if v == nil {
		return nil
	}
	w := To(*v)
	return &w
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
