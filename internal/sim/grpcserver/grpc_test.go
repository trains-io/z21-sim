package grpcserver_test

import (
	"context"
	"testing"
	"time"

	simctlv1 "github.com/trains-io/z21-sim/api/simctl/v1"
	"github.com/trains-io/z21-sim/internal/sim"
	"github.com/trains-io/z21-sim/internal/sim/grpcserver"
	"github.com/trains-io/z21.go/client"
	"github.com/trains-io/z21.go/protocol"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func startSimulator(t *testing.T) (*sim.Server, string) {
	t.Helper()

	simulator, err := sim.New("127.0.0.1:0", nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- simulator.Serve(ctx)
	}()

	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Log("simulator did not stop cleanly")
		}
		_ = simulator.Close()
	})

	return simulator, simulator.Addr().String()
}

func TestGRPCBroadcastSystemState(t *testing.T) {
	simulator, udpAddr := startSimulator(t)

	grpcSrv, err := grpcserver.New(simulator, "127.0.0.1:0", nil)
	if err != nil {
		t.Fatalf("grpcserver.New: %v", err)
	}
	t.Cleanup(grpcSrv.Stop)

	go func() {
		_ = grpcSrv.Serve()
	}()

	c, err := client.DialLocal(udpAddr, 42120)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Send(ctx, protocol.SetBroadcastFlags(protocol.BroadcastFlagSystemState)); err != nil {
		t.Fatalf("SetBroadcastFlags: %v", err)
	}
	if _, err := c.Call(ctx, protocol.GetBroadcastFlags()); err != nil {
		t.Fatalf("GetBroadcastFlags: %v", err)
	}

	readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
	defer readCancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		grpcCtx, grpcCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer grpcCancel()

		conn, err := grpc.NewClient(
			grpcSrv.Addr().String(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			t.Errorf("grpc.NewClient: %v", err)
			return
		}
		defer conn.Close()

		_, err = simctlv1.NewSimulatorServiceClient(conn).BroadcastSystemState(grpcCtx, &simctlv1.BroadcastSystemStateRequest{})
		if err != nil {
			t.Errorf("BroadcastSystemState: %v", err)
		}
	}()

	msgs, err := c.ReadPacket(readCtx)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	if !containsSystemStateBroadcast(msgs) {
		t.Fatalf("expected system state broadcast, got %#v", msgs)
	}
}

func TestGRPCSetTrackPower(t *testing.T) {
	simulator, udpAddr := startSimulator(t)

	grpcSrv, err := grpcserver.New(simulator, "127.0.0.1:0", nil)
	if err != nil {
		t.Fatalf("grpcserver.New: %v", err)
	}
	t.Cleanup(grpcSrv.Stop)

	go func() {
		_ = grpcSrv.Serve()
	}()

	conn, err := grpc.NewClient(
		grpcSrv.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	defer conn.Close()

	grpcClient := simctlv1.NewSimulatorServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := grpcClient.SetTrackPower(ctx, &simctlv1.SetTrackPowerRequest{On: false})
	if err != nil {
		t.Fatalf("SetTrackPower: %v", err)
	}
	if resp.GetOn() {
		t.Fatalf("resp.On = true, want false")
	}
	if !resp.GetChanged() {
		t.Fatal("resp.Changed = false, want true")
	}

	c, err := client.Dial(udpAddr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	msgs, err := c.Call(ctx, protocol.GetXStatus())
	if err != nil {
		t.Fatalf("GetXStatus: %v", err)
	}
	status, err := protocol.XStatusFromMessages(msgs)
	if err != nil {
		t.Fatalf("XStatusFromMessages: %v", err)
	}
	if got := protocol.FormatXStatusFlags(status.CentralState); got != "track voltage off" {
		t.Fatalf("CentralState = %q, want track voltage off", got)
	}
}

func TestGRPCNotifyCANDeviceDetected(t *testing.T) {
	simulator, udpAddr := startSimulator(t)

	grpcSrv, err := grpcserver.New(simulator, "127.0.0.1:0", nil)
	if err != nil {
		t.Fatalf("grpcserver.New: %v", err)
	}
	t.Cleanup(grpcSrv.Stop)

	go func() {
		_ = grpcSrv.Serve()
	}()

	c, err := client.DialLocal(udpAddr, 42131)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Send(ctx, protocol.SetBroadcastFlags(protocol.BroadcastFlagCANDetector)); err != nil {
		t.Fatalf("SetBroadcastFlags: %v", err)
	}
	if _, err := c.Call(ctx, protocol.GetBroadcastFlags()); err != nil {
		t.Fatalf("GetBroadcastFlags: %v", err)
	}

	readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
	defer readCancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		grpcCtx, grpcCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer grpcCancel()

		conn, err := grpc.NewClient(
			grpcSrv.Addr().String(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			t.Errorf("grpc.NewClient: %v", err)
			return
		}
		defer conn.Close()

		_, err = simctlv1.NewSimulatorServiceClient(conn).NotifyCANDeviceDetected(grpcCtx, &simctlv1.NotifyCANDeviceDetectedRequest{
			NetId:      0xDB04,
			Kind:       simctlv1.CANDeviceKind_CAN_DEVICE_KIND_DETECTOR,
			Name:       "Det 4",
			ModuleAddr: 60,
			PortCount:  1,
		})
		if err != nil {
			t.Errorf("NotifyCANDeviceDetected: %v", err)
		}
	}()

	msgs, err := c.ReadPacket(readCtx)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	reports, err := protocol.CANDetectorReportsFromMessages(msgs)
	if err != nil {
		t.Fatalf("CANDetectorReportsFromMessages: %v", err)
	}
	if len(reports) != 1 || reports[0].NetID != 0xDB04 || reports[0].Addr != 60 {
		t.Fatalf("reports = %+v", reports)
	}
}

// startControl serves the control API for simulator and returns a connected
// client.
func startControl(t *testing.T, simulator *sim.Server) simctlv1.SimulatorServiceClient {
	t.Helper()
	grpcSrv, err := grpcserver.New(simulator, "127.0.0.1:0", nil)
	if err != nil {
		t.Fatalf("grpcserver.New: %v", err)
	}
	t.Cleanup(grpcSrv.Stop)
	go func() { _ = grpcSrv.Serve() }()

	conn, err := grpc.NewClient(grpcSrv.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return simctlv1.NewSimulatorServiceClient(conn)
}

func TestGRPCInjectRaw(t *testing.T) {
	simulator, udpAddr := startSimulator(t)
	control := startControl(t, simulator)

	c, err := client.Dial(udpAddr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Send(ctx, protocol.SetBroadcastFlags(protocol.BroadcastFlagXpressNet)); err != nil {
		t.Fatalf("SetBroadcastFlags: %v", err)
	}
	if _, err := c.Call(ctx, protocol.GetBroadcastFlags()); err != nil {
		t.Fatalf("GetBroadcastFlags: %v", err)
	}

	payload, err := protocol.MarshalAll(protocol.EncodeTrackPowerBC(false))
	if err != nil {
		t.Fatalf("MarshalAll: %v", err)
	}
	resp, err := control.InjectRaw(ctx, &simctlv1.InjectRawRequest{
		Payload:        payload,
		BroadcastFlags: protocol.BroadcastFlagXpressNet,
	})
	if err != nil {
		t.Fatalf("InjectRaw: %v", err)
	}
	if resp.GetRecipientCount() != 1 {
		t.Fatalf("RecipientCount = %d, want 1", resp.GetRecipientCount())
	}

	msgs, err := c.ReadPacket(ctx)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %#v, want the injected dataset", msgs)
	}
	if on, err := protocol.ParseTrackPowerBC(msgs[0].Data); err != nil || on {
		t.Fatalf("injected dataset = %#v, want track power off", msgs[0])
	}
	replies, err := c.Call(ctx, protocol.GetXStatus())
	if err != nil {
		t.Fatalf("GetXStatus: %v", err)
	}
	if status, err := protocol.XStatusFromMessages(replies); err != nil || status.CentralState != 0 {
		t.Fatalf("X status after InjectRaw = %+v, err=%v; want unchanged (track power on)", status, err)
	}
}

func TestGRPCInjectRawInvalid(t *testing.T) {
	simulator, _ := startSimulator(t)
	control := startControl(t, simulator)

	valid, err := protocol.MarshalAll(protocol.EncodeTrackPowerBC(false))
	if err != nil {
		t.Fatalf("MarshalAll: %v", err)
	}
	tests := []struct {
		name string
		req  *simctlv1.InjectRawRequest
	}{
		{name: "zero flags", req: &simctlv1.InjectRawRequest{Payload: valid}},
		{name: "truncated payload", req: &simctlv1.InjectRawRequest{Payload: valid[:3], BroadcastFlags: 1}},
		{name: "empty payload", req: &simctlv1.InjectRawRequest{BroadcastFlags: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := control.InjectRaw(ctx, tt.req)
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("InjectRaw error = %v, want InvalidArgument", err)
			}
		})
	}
}

func TestGRPCListClients(t *testing.T) {
	simulator, udpAddr := startSimulator(t)
	control := startControl(t, simulator)

	c, err := client.Dial(udpAddr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	before := time.Now()
	if err := c.SendMany(ctx, protocol.SetBroadcastFlags(protocol.BroadcastFlagXpressNet), protocol.GetLocoInfo(3)); err != nil {
		t.Fatalf("SendMany: %v", err)
	}
	if _, err := c.Call(ctx, protocol.GetBroadcastFlags()); err != nil {
		t.Fatalf("GetBroadcastFlags: %v", err)
	}

	resp, err := control.ListClients(ctx, &simctlv1.ListClientsRequest{})
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	if len(resp.GetClients()) != 1 {
		t.Fatalf("clients = %v, want 1", resp.GetClients())
	}
	got := resp.GetClients()[0]
	if got.GetAddr() != c.LocalAddr().String() || got.GetLocalAddr() != udpAddr {
		t.Fatalf("addr = %q, local_addr = %q; want %q, %q", got.GetAddr(), got.GetLocalAddr(), c.LocalAddr(), udpAddr)
	}
	if got.GetBroadcastFlags() != protocol.BroadcastFlagXpressNet {
		t.Fatalf("broadcast_flags = %#08x, want X-Bus", got.GetBroadcastFlags())
	}
	if locos := got.GetLocos(); len(locos) != 1 || locos[0] != 3 {
		t.Fatalf("locos = %v, want [3]", locos)
	}
	if seen := got.GetLastSeen().AsTime(); seen.Before(before.Add(-time.Second)) || seen.After(time.Now()) {
		t.Fatalf("last_seen = %v, want between %v and now", seen, before)
	}

	if err := c.Send(ctx, protocol.Message{Header: protocol.HeaderLANLogoff}); err != nil {
		t.Fatalf("Send LAN_LOGOFF: %v", err)
	}
	for {
		resp, err := control.ListClients(ctx, &simctlv1.ListClientsRequest{})
		if err != nil {
			t.Fatalf("ListClients: %v", err)
		}
		if len(resp.GetClients()) == 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("client still listed after LAN_LOGOFF: %v", resp.GetClients())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestGRPCSetSystemState(t *testing.T) {
	simulator, udpAddr := startSimulator(t)
	control := startControl(t, simulator)

	c, err := client.Dial(udpAddr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Send(ctx, protocol.SetBroadcastFlags(protocol.BroadcastFlagSystemState)); err != nil {
		t.Fatalf("SetBroadcastFlags: %v", err)
	}
	if _, err := c.Call(ctx, protocol.GetBroadcastFlags()); err != nil {
		t.Fatalf("GetBroadcastFlags: %v", err)
	}

	temperature, supply := int32(-5), uint32(18000)
	resp, err := control.SetSystemState(ctx, &simctlv1.SetSystemStateRequest{TemperatureC: &temperature, SupplyVoltageMv: &supply})
	if err != nil {
		t.Fatalf("SetSystemState: %v", err)
	}
	if !resp.GetChanged() || resp.GetRecipientCount() != 1 {
		t.Fatalf("response = %v, want changed and 1 recipient", resp)
	}

	msgs, err := c.ReadPacket(ctx)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	state, err := protocol.SystemStateFromMessages(msgs)
	if err != nil {
		t.Fatalf("SystemStateFromMessages: %v", err)
	}
	if state.Temperature != -5 || state.SupplyVoltage != 18000 || state.MainCurrent != 100 {
		t.Fatalf("system state = %+v, want temperature -5, supply 18000, main current unchanged 100", state)
	}

	resp, err = control.SetSystemState(ctx, &simctlv1.SetSystemStateRequest{})
	if err != nil {
		t.Fatalf("SetSystemState without values: %v", err)
	}
	if resp.GetChanged() || resp.GetRecipientCount() != 0 {
		t.Fatalf("response without values = %v, want unchanged and no recipients", resp)
	}
}

func TestGRPCSetSystemStateOutOfRange(t *testing.T) {
	simulator, _ := startSimulator(t)
	control := startControl(t, simulator)

	tooHot, tooHigh, ok := int32(40000), uint32(70000), int32(250)
	tests := []struct {
		name string
		req  *simctlv1.SetSystemStateRequest
	}{
		{name: "int16 field", req: &simctlv1.SetSystemStateRequest{TemperatureC: &tooHot}},
		{name: "uint16 field", req: &simctlv1.SetSystemStateRequest{VccVoltageMv: &tooHigh}},
		{name: "valid and invalid together", req: &simctlv1.SetSystemStateRequest{MainCurrentMa: &ok, SupplyVoltageMv: &tooHigh}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := control.SetSystemState(ctx, tt.req); status.Code(err) != codes.InvalidArgument {
				t.Fatalf("SetSystemState error = %v, want InvalidArgument", err)
			}
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := control.SetSystemState(ctx, &simctlv1.SetSystemStateRequest{MainCurrentMa: &ok})
	if err != nil || !resp.GetChanged() {
		t.Fatalf("SetSystemState(main current 250) = %v, %v; want changed (rejected requests must not apply)", resp, err)
	}
}

func containsSystemStateBroadcast(msgs []protocol.Message) bool {
	for _, msg := range msgs {
		if protocol.IsSystemStateDataChanged(msg) {
			return true
		}
	}
	return false
}
