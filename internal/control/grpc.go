package control

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	controlv1 "github.com/trains-io/z21-sim-go/api/control/v1"
	"github.com/trains-io/z21-sim-go/internal/server"
	"google.golang.org/grpc"
)

// GRPCServer exposes simulator control RPCs to sidecar containers.
type GRPCServer struct {
	controlv1.UnimplementedSimulatorServiceServer

	sim *server.Server
	log *slog.Logger
	srv *grpc.Server
	lis net.Listener
}

// NewGRPCServer listens on addr and serves the control API for sim.
func NewGRPCServer(sim *server.Server, addr string, log *slog.Logger) (*GRPCServer, error) {
	if sim == nil {
		return nil, fmt.Errorf("control: nil simulator")
	}
	if log == nil {
		log = slog.Default()
	}

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("control: listen %s: %w", addr, err)
	}

	srv := grpc.NewServer()
	g := &GRPCServer{sim: sim, log: log, srv: srv, lis: lis}
	controlv1.RegisterSimulatorServiceServer(srv, g)
	return g, nil
}

// Addr returns the bound TCP address.
func (g *GRPCServer) Addr() net.Addr {
	if g == nil || g.lis == nil {
		return nil
	}
	return g.lis.Addr()
}

// Serve blocks until the gRPC server stops.
func (g *GRPCServer) Serve() error {
	if g == nil || g.srv == nil || g.lis == nil {
		return fmt.Errorf("control: not configured")
	}
	g.log.Info("gRPC control listening", "addr", g.lis.Addr())
	return g.srv.Serve(g.lis)
}

// Stop gracefully stops the gRPC server.
func (g *GRPCServer) Stop() {
	if g == nil || g.srv == nil {
		return
	}
	g.srv.GracefulStop()
}

func (g *GRPCServer) BroadcastSystemState(context.Context, *controlv1.BroadcastSystemStateRequest) (*controlv1.BroadcastSystemStateResponse, error) {
	count := g.sim.BroadcastSystemState()
	return &controlv1.BroadcastSystemStateResponse{RecipientCount: int32(count)}, nil
}

func (g *GRPCServer) SetTrackPower(_ context.Context, req *controlv1.SetTrackPowerRequest) (*controlv1.SetTrackPowerResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("control: nil request")
	}
	changed := g.sim.SetTrackPowerAndNotify(req.GetOn())
	return &controlv1.SetTrackPowerResponse{
		On:      req.GetOn(),
		Changed: changed,
	}, nil
}

func (g *GRPCServer) NotifyCANDeviceDetected(_ context.Context, req *controlv1.NotifyCANDeviceDetectedRequest) (*controlv1.NotifyCANDeviceDetectedResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("control: nil request")
	}

	kind, err := canDeviceKindFromProto(req.GetKind())
	if err != nil {
		return nil, err
	}

	device, err := server.BuildCANDevice(
		req.GetNetId(),
		kind,
		req.GetName(),
		req.GetModuleAddr(),
		req.GetPortCount(),
		req.GetOutputPort(),
	)
	if err != nil {
		return nil, err
	}

	count := g.sim.NotifyCANDeviceDetected(device)
	return &controlv1.NotifyCANDeviceDetectedResponse{RecipientCount: int32(count)}, nil
}

func canDeviceKindFromProto(kind controlv1.CANDeviceKind) (server.CANDeviceKind, error) {
	switch kind {
	case controlv1.CANDeviceKind_CAN_DEVICE_KIND_DETECTOR:
		return server.CANDeviceDetector, nil
	case controlv1.CANDeviceKind_CAN_DEVICE_KIND_BOOSTER:
		return server.CANDeviceBooster, nil
	default:
		return 0, fmt.Errorf("control: kind is required")
	}
}
