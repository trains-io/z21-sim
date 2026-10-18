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
	"google.golang.org/protobuf/proto"
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

func TestGRPCEmergencyStop(t *testing.T) {
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

	for i, wantChanged := range []bool{true, false} {
		resp, err := control.EmergencyStop(ctx, &simctlv1.EmergencyStopRequest{})
		if err != nil {
			t.Fatalf("EmergencyStop #%d: %v", i+1, err)
		}
		if resp.GetChanged() != wantChanged {
			t.Fatalf("EmergencyStop #%d changed = %v, want %v", i+1, resp.GetChanged(), wantChanged)
		}
	}

	msgs, err := c.ReadPacket(ctx)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	if err := protocol.BCStoppedFromMessages(msgs); err != nil {
		t.Fatalf("subscriber got %#v: %v", msgs, err)
	}
}

func TestGRPCShortCircuit(t *testing.T) {
	simulator, udpAddr := startSimulator(t)
	control := startControl(t, simulator)

	c, err := client.Dial(udpAddr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Send(ctx, protocol.SetBroadcastFlags(protocol.BroadcastFlagXpressNet|protocol.BroadcastFlagSystemState)); err != nil {
		t.Fatalf("SetBroadcastFlags: %v", err)
	}
	if _, err := c.Call(ctx, protocol.GetBroadcastFlags()); err != nil {
		t.Fatalf("GetBroadcastFlags: %v", err)
	}

	tests := []struct {
		kind        simctlv1.ShortCircuitKind
		wantChanged bool
	}{
		{kind: simctlv1.ShortCircuitKind_SHORT_CIRCUIT_KIND_EXTERNAL, wantChanged: true},
		{kind: simctlv1.ShortCircuitKind_SHORT_CIRCUIT_KIND_EXTERNAL, wantChanged: false},
	}
	for i, tt := range tests {
		resp, err := control.ShortCircuit(ctx, &simctlv1.ShortCircuitRequest{Kind: tt.kind})
		if err != nil {
			t.Fatalf("ShortCircuit #%d: %v", i+1, err)
		}
		if resp.GetChanged() != tt.wantChanged {
			t.Fatalf("ShortCircuit #%d changed = %v, want %v", i+1, resp.GetChanged(), tt.wantChanged)
		}
	}

	var msgs []protocol.Message
	for !containsSystemStateBroadcast(msgs) {
		packet, err := c.ReadPacket(ctx)
		if err != nil {
			t.Fatalf("ReadPacket: %v (got %#v)", err, msgs)
		}
		msgs = append(msgs, packet...)
	}
	var shorts int
	for _, msg := range msgs {
		if protocol.IsBCShortCircuit(msg.Data) {
			shorts++
		}
	}
	if shorts != 1 {
		t.Fatalf("subscriber got %#v, want LAN_X_BC_TRACK_SHORT_CIRCUIT once", msgs)
	}
	state, err := protocol.SystemStateFromMessages(msgs)
	if err != nil || state.CentralStateEx != protocol.CentralStateExShortCircuitExternal {
		t.Fatalf("system state = %+v, err=%v; want external short circuit", state, err)
	}

	if _, err := control.ShortCircuit(ctx, &simctlv1.ShortCircuitRequest{Kind: 99}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("ShortCircuit(kind 99) error = %v, want InvalidArgument", err)
	}
}

func TestGRPCNotifyCANDeviceDetectedInvalid(t *testing.T) {
	simulator, _ := startSimulator(t)
	control := startControl(t, simulator)

	tests := []struct {
		name string
		req  *simctlv1.NotifyCANDeviceDetectedRequest
	}{
		{name: "kind unspecified", req: &simctlv1.NotifyCANDeviceDetectedRequest{NetId: 1}},
		{name: "net_id zero", req: &simctlv1.NotifyCANDeviceDetectedRequest{Kind: simctlv1.CANDeviceKind_CAN_DEVICE_KIND_BOOSTER}},
		{name: "port_count too large", req: &simctlv1.NotifyCANDeviceDetectedRequest{NetId: 1, Kind: simctlv1.CANDeviceKind_CAN_DEVICE_KIND_DETECTOR, PortCount: 257}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := control.NotifyCANDeviceDetected(ctx, tt.req); status.Code(err) != codes.InvalidArgument {
				t.Fatalf("NotifyCANDeviceDetected error = %v, want InvalidArgument", err)
			}
		})
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

func TestGRPCSetLoco(t *testing.T) {
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
	if _, err := c.Call(ctx, protocol.GetLocoInfo(3)); err != nil {
		t.Fatalf("GetLocoInfo: %v", err)
	}

	steps, speed, forward := simctlv1.LocoSpeedSteps_LOCO_SPEED_STEPS_28, uint32(2), false
	req := &simctlv1.SetLocoRequest{
		Address:    3,
		SpeedSteps: &steps,
		Speed:      &speed,
		Forward:    &forward,
		Functions:  map[uint32]bool{0: true, 31: true},
	}
	resp, err := control.SetLoco(ctx, req)
	if err != nil {
		t.Fatalf("SetLoco: %v", err)
	}
	wantLoco := &simctlv1.Loco{Address: 3, SpeedSteps: steps, Speed: 2, Functions: 1 | 1<<31}
	if !proto.Equal(resp.GetLoco(), wantLoco) || !resp.GetChanged() || resp.GetRecipientCount() != 1 {
		t.Fatalf("SetLoco = %v, want loco %v, changed, 1 recipient", resp, wantLoco)
	}

	msgs, err := c.ReadPacket(ctx)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	info, err := protocol.LocoInfoFromMessages(msgs)
	if err != nil {
		t.Fatalf("subscriber got %#v: %v", msgs, err)
	}
	want := protocol.LocoInfo{Address: 3, SpeedSteps: protocol.LocoInfoSpeedSteps28, Speed: 0x12, Headlight: true, FunctionsF29F31: 0x04}
	if info != want {
		t.Fatalf("LAN_X_LOCO_INFO = %+v, want %+v", info, want)
	}

	resp, err = control.SetLoco(ctx, req)
	if err != nil {
		t.Fatalf("SetLoco again: %v", err)
	}
	if resp.GetChanged() || resp.GetRecipientCount() != 0 || !proto.Equal(resp.GetLoco(), wantLoco) {
		t.Fatalf("SetLoco again = %v, want loco %v, unchanged, no recipients", resp, wantLoco)
	}
}

func TestGRPCSetLocoInvalid(t *testing.T) {
	simulator, _ := startSimulator(t)
	control := startControl(t, simulator)

	tooFast, beyondUint8 := uint32(127), uint32(300)
	unknownSteps := simctlv1.LocoSpeedSteps(99)
	tests := []struct {
		name string
		req  *simctlv1.SetLocoRequest
	}{
		{name: "address 0", req: &simctlv1.SetLocoRequest{}},
		{name: "address 10000", req: &simctlv1.SetLocoRequest{Address: 10000}},
		{name: "address beyond uint16", req: &simctlv1.SetLocoRequest{Address: 65539}},
		{name: "unknown speed steps", req: &simctlv1.SetLocoRequest{Address: 3, SpeedSteps: &unknownSteps}},
		{name: "speed above 126", req: &simctlv1.SetLocoRequest{Address: 3, Speed: &tooFast}},
		{name: "speed beyond uint8", req: &simctlv1.SetLocoRequest{Address: 3, Speed: &beyondUint8}},
		{name: "F32", req: &simctlv1.SetLocoRequest{Address: 3, Functions: map[uint32]bool{32: true}}},
		{name: "F300", req: &simctlv1.SetLocoRequest{Address: 3, Functions: map[uint32]bool{300: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := control.SetLoco(ctx, tt.req); status.Code(err) != codes.InvalidArgument {
				t.Fatalf("SetLoco error = %v, want InvalidArgument", err)
			}
		})
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
