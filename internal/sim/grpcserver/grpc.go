package grpcserver

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	simctlv1 "github.com/trains-io/z21-sim/api/simctl/v1"
	"github.com/trains-io/z21-sim/internal/sim"
	"github.com/trains-io/z21-sim/internal/trace"
	"google.golang.org/grpc"
)

// GRPCServer exposes simulator control RPCs to sidecar containers.
type GRPCServer struct {
	simctlv1.UnimplementedSimulatorServiceServer

	sim    *sim.Server
	log    *slog.Logger
	tracer *trace.Tracer
	srv    *grpc.Server
	lis    net.Listener
}

// NewGRPCServer listens on addr and serves the control API for simulator.
func NewGRPCServer(simulator *sim.Server, addr string, log *slog.Logger, opts ...Option) (*GRPCServer, error) {
	if simulator == nil {
		return nil, fmt.Errorf("control: nil simulator")
	}
	if log == nil {
		log = slog.Default()
	}

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("control: listen %s: %w", addr, err)
	}

	g := &GRPCServer{sim: simulator, log: log, lis: lis}
	applyOptions(g, opts)

	var serverOpts []grpc.ServerOption
	if g.tracer != nil {
		serverOpts = append(serverOpts, grpc.UnaryInterceptor(grpcTraceInterceptor(g.tracer)))
	}
	srv := grpc.NewServer(serverOpts...)
	g.srv = srv
	simctlv1.RegisterSimulatorServiceServer(srv, g)
	return g, nil
}

func applyOptions(g *GRPCServer, opts []Option) {
	for _, opt := range opts {
		if opt != nil {
			opt(g)
		}
	}
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

func (g *GRPCServer) BroadcastSystemState(context.Context, *simctlv1.BroadcastSystemStateRequest) (*simctlv1.BroadcastSystemStateResponse, error) {
	count := g.sim.BroadcastSystemState()
	return &simctlv1.BroadcastSystemStateResponse{RecipientCount: int32(count)}, nil
}

func (g *GRPCServer) SetTrackPower(_ context.Context, req *simctlv1.SetTrackPowerRequest) (*simctlv1.SetTrackPowerResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("control: nil request")
	}
	changed := g.sim.SetTrackPowerAndNotify(req.GetOn())
	return &simctlv1.SetTrackPowerResponse{
		On:      req.GetOn(),
		Changed: changed,
	}, nil
}

func (g *GRPCServer) NotifyCANDeviceDetected(_ context.Context, req *simctlv1.NotifyCANDeviceDetectedRequest) (*simctlv1.NotifyCANDeviceDetectedResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("control: nil request")
	}

	kind, err := canDeviceKindFromProto(req.GetKind())
	if err != nil {
		return nil, err
	}

	device, err := sim.BuildCANDevice(
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
	return &simctlv1.NotifyCANDeviceDetectedResponse{RecipientCount: int32(count)}, nil
}

func canDeviceKindFromProto(kind simctlv1.CANDeviceKind) (sim.CANDeviceKind, error) {
	switch kind {
	case simctlv1.CANDeviceKind_CAN_DEVICE_KIND_DETECTOR:
		return sim.CANDeviceDetector, nil
	case simctlv1.CANDeviceKind_CAN_DEVICE_KIND_BOOSTER:
		return sim.CANDeviceBooster, nil
	default:
		return 0, fmt.Errorf("control: kind is required")
	}
}
