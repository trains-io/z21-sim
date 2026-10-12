package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"

	simctlv1 "github.com/trains-io/z21-sim/api/simctl/v1"
	"github.com/trains-io/z21-sim/internal/sim"
	"github.com/trains-io/z21-sim/internal/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Server exposes simulator control RPCs to sidecar containers.
type Server struct {
	simctlv1.UnimplementedSimulatorServiceServer

	sim    *sim.Server
	log    *slog.Logger
	tracer *trace.Tracer
	srv    *grpc.Server
	lis    net.Listener
}

// New listens on addr and serves the control API for simulator.
func New(simulator *sim.Server, addr string, log *slog.Logger, opts ...Option) (*Server, error) {
	if simulator == nil {
		return nil, fmt.Errorf("grpcserver: nil simulator")
	}
	if log == nil {
		log = slog.Default()
	}

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("grpcserver: listen %s: %w", addr, err)
	}

	g := &Server{sim: simulator, log: log, lis: lis}
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

func applyOptions(g *Server, opts []Option) {
	for _, opt := range opts {
		if opt != nil {
			opt(g)
		}
	}
}

// Addr returns the bound TCP address.
func (g *Server) Addr() net.Addr {
	if g == nil || g.lis == nil {
		return nil
	}
	return g.lis.Addr()
}

// Serve blocks until the gRPC server stops.
func (g *Server) Serve() error {
	if g == nil || g.srv == nil || g.lis == nil {
		return fmt.Errorf("grpcserver: not configured")
	}
	g.log.Info("gRPC control listening", "addr", g.lis.Addr())
	return g.srv.Serve(g.lis)
}

// Stop gracefully stops the gRPC server.
func (g *Server) Stop() {
	if g == nil || g.srv == nil {
		return
	}
	g.srv.GracefulStop()
}

func (g *Server) BroadcastSystemState(context.Context, *simctlv1.BroadcastSystemStateRequest) (*simctlv1.BroadcastSystemStateResponse, error) {
	count := g.sim.BroadcastSystemState()
	return &simctlv1.BroadcastSystemStateResponse{RecipientCount: int32(count)}, nil
}

func (g *Server) SetTrackPower(_ context.Context, req *simctlv1.SetTrackPowerRequest) (*simctlv1.SetTrackPowerResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("grpcserver: nil request")
	}
	changed := g.sim.SetTrackPowerAndNotify(req.GetOn())
	return &simctlv1.SetTrackPowerResponse{
		On:      req.GetOn(),
		Changed: changed,
	}, nil
}

func (g *Server) NotifyCANDeviceDetected(_ context.Context, req *simctlv1.NotifyCANDeviceDetectedRequest) (*simctlv1.NotifyCANDeviceDetectedResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("grpcserver: nil request")
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

func (g *Server) InjectRaw(_ context.Context, req *simctlv1.InjectRawRequest) (*simctlv1.InjectRawResponse, error) {
	count, err := g.sim.InjectRaw(req.GetBroadcastFlags(), req.GetPayload())
	if errors.Is(err, sim.ErrInvalidInjection) {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err != nil {
		return nil, err
	}
	return &simctlv1.InjectRawResponse{RecipientCount: int32(count)}, nil
}

func (g *Server) ListClients(context.Context, *simctlv1.ListClientsRequest) (*simctlv1.ListClientsResponse, error) {
	clients := g.sim.Clients()
	resp := &simctlv1.ListClientsResponse{Clients: make([]*simctlv1.Client, 0, len(clients))}
	for _, c := range clients {
		locos := make([]uint32, len(c.Locos))
		for i, addr := range c.Locos {
			locos[i] = uint32(addr)
		}
		resp.Clients = append(resp.Clients, &simctlv1.Client{
			Addr:           c.Addr.String(),
			LocalAddr:      c.LocalAddr.String(),
			BroadcastFlags: c.BroadcastFlags,
			Locos:          locos,
			LastSeen:       timestamppb.New(c.LastSeen),
		})
	}
	return resp, nil
}

func (g *Server) SetSystemState(_ context.Context, req *simctlv1.SetSystemStateRequest) (*simctlv1.SetSystemStateResponse, error) {
	var (
		v    sim.SystemValues
		errs []error
	)
	v.MainCurrent, errs = int16Field(errs, "main_current_ma", req.MainCurrentMa)
	v.ProgCurrent, errs = int16Field(errs, "prog_current_ma", req.ProgCurrentMa)
	v.FilteredMainCurrent, errs = int16Field(errs, "filtered_main_current_ma", req.FilteredMainCurrentMa)
	v.Temperature, errs = int16Field(errs, "temperature_c", req.TemperatureC)
	v.SupplyVoltage, errs = uint16Field(errs, "supply_voltage_mv", req.SupplyVoltageMv)
	v.VCCVoltage, errs = uint16Field(errs, "vcc_voltage_mv", req.VccVoltageMv)
	if err := errors.Join(errs...); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	changed, count := g.sim.SetSystemValues(v)
	return &simctlv1.SetSystemStateResponse{Changed: changed, RecipientCount: int32(count)}, nil
}

func (g *Server) EmergencyStop(context.Context, *simctlv1.EmergencyStopRequest) (*simctlv1.EmergencyStopResponse, error) {
	return &simctlv1.EmergencyStopResponse{Changed: g.sim.EmergencyStop()}, nil
}

func (g *Server) ShortCircuit(_ context.Context, req *simctlv1.ShortCircuitRequest) (*simctlv1.ShortCircuitResponse, error) {
	var kind sim.ShortCircuitKind
	switch req.GetKind() {
	case simctlv1.ShortCircuitKind_SHORT_CIRCUIT_KIND_UNSPECIFIED, simctlv1.ShortCircuitKind_SHORT_CIRCUIT_KIND_INTERNAL:
		kind = sim.ShortCircuitInternal
	case simctlv1.ShortCircuitKind_SHORT_CIRCUIT_KIND_EXTERNAL:
		kind = sim.ShortCircuitExternal
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unknown short circuit kind %d", req.GetKind())
	}
	return &simctlv1.ShortCircuitResponse{Changed: g.sim.ShortCircuit(kind)}, nil
}

func int16Field(errs []error, name string, v *int32) (*int16, []error) {
	if v == nil {
		return nil, errs
	}
	if *v < math.MinInt16 || *v > math.MaxInt16 {
		return nil, append(errs, fmt.Errorf("%s %d out of range [%d, %d]", name, *v, math.MinInt16, math.MaxInt16))
	}
	n := int16(*v)
	return &n, errs
}

func uint16Field(errs []error, name string, v *uint32) (*uint16, []error) {
	if v == nil {
		return nil, errs
	}
	if *v > math.MaxUint16 {
		return nil, append(errs, fmt.Errorf("%s %d out of range [0, %d]", name, *v, math.MaxUint16))
	}
	n := uint16(*v)
	return &n, errs
}

func canDeviceKindFromProto(kind simctlv1.CANDeviceKind) (sim.CANDeviceKind, error) {
	switch kind {
	case simctlv1.CANDeviceKind_CAN_DEVICE_KIND_DETECTOR:
		return sim.CANDeviceDetector, nil
	case simctlv1.CANDeviceKind_CAN_DEVICE_KIND_BOOSTER:
		return sim.CANDeviceBooster, nil
	default:
		return 0, fmt.Errorf("grpcserver: kind is required")
	}
}
