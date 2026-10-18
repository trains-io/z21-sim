package sim

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"

	"github.com/trains-io/z21.go/protocol"
	"github.com/trains-io/z21.go/server"
)

// Server simulates a Z21 command station on top of z21.go/server, which owns
// the UDP transport, client sessions and broadcasts.
type Server struct {
	z21             *server.Server
	log             *slog.Logger
	traceOut        io.Writer
	additionalAddrs []string
	identity        Identity
	state           *State
}

// New binds a UDP listener on addr (for example "0.0.0.0:21105" or "127.0.0.1:0").
func New(addr string, log *slog.Logger, opts ...Option) (*Server, error) {
	if log == nil {
		log = slog.Default()
	}

	s := &Server{
		log:      log,
		identity: DefaultIdentity(),
	}
	applyOptions(s, opts)
	s.state = newState(s.identity.Capabilities)

	serverOpts := []server.Option{server.WithHandler(s.mux()), server.WithLogger(log)}
	if s.traceOut != nil {
		serverOpts = append(serverOpts, server.WithObserver(server.NewTracer(s.traceOut)))
	}
	for _, a := range s.additionalAddrs {
		serverOpts = append(serverOpts, server.WithAdditionalAddr(a))
	}
	z21, err := server.New(addr, serverOpts...)
	if err != nil {
		return nil, fmt.Errorf("z21-sim: %w", err)
	}
	s.z21 = z21
	return s, nil
}

func applyOptions(s *Server, opts []Option) {
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
}

// Addr returns the bound local UDP address.
func (s *Server) Addr() net.Addr {
	if s == nil || s.z21 == nil {
		return nil
	}
	return s.z21.Addr()
}

// Addrs returns the local addresses of all UDP sockets, starting with Addr.
func (s *Server) Addrs() []net.Addr {
	if s == nil || s.z21 == nil {
		return nil
	}
	return s.z21.Addrs()
}

// Clients returns the Z21 LAN clients with an active session, ordered by
// address.
func (s *Server) Clients() []server.ClientInfo {
	return s.z21.Clients()
}

// Close releases the UDP sockets.
func (s *Server) Close() error {
	if s == nil || s.z21 == nil {
		return nil
	}
	return s.z21.Close()
}

// Serve handles incoming datagrams until ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	if s == nil || s.z21 == nil {
		return fmt.Errorf("z21-sim: not listening")
	}
	return s.z21.Serve(ctx)
}

// mux routes the requests the simulator implements. Session requests
// (broadcast flags, logoff) are handled by z21.go/server itself.
func (s *Server) mux() *server.Mux {
	m := server.NewMux()
	m.Handle(protocol.HeaderLANGetHWInfo, reply(s.hwInfoReply))
	m.Handle(protocol.HeaderLANGetSerialNumber, reply(s.serialReply))
	m.Handle(protocol.HeaderLANGetCode, reply(s.codeReply))
	m.Handle(protocol.HeaderLANSystemStateGetData, reply(s.systemStateReply))

	m.HandleLANXDB0(xHeaderSystem, xCommandGetVersion, reply(xVersionReply))
	m.HandleLANXDB0(xHeaderSystem, xCommandGetStatus, reply(s.xStatusReply))
	m.HandleLANXDB0(xHeaderSystem, xCommandSetTrackPowerOff, s.trackPowerHandler(false))
	m.HandleLANXDB0(xHeaderSystem, xCommandSetTrackPowerOn, s.trackPowerHandler(true))
	m.HandleLANX(xHeaderSetStop, s.stopHandler())
	m.HandleLANXDB0(xCommandGetFirmware, xFirmwareDB0, reply(s.xFirmwareReply))

	m.HandleLANXDB0(xHeaderGetLocoInfo, xCommandGetLocoInfo, s.locoInfoHandler())
	m.HandleLANXDB0(xHeaderGetLocoInfo, xCommandPurgeLoco, s.purgeLocoHandler())
	for _, steps := range []byte{protocol.LocoSpeedSteps14, protocol.LocoSpeedSteps28, protocol.LocoSpeedSteps128} {
		m.HandleLANXDB0(xHeaderSetLocoDrive, 0x10|steps, s.locoDriveHandler())
	}
	m.HandleLANXDB0(xHeaderSetLocoDrive, xCommandSetLocoFunction, s.locoFunctionHandler())
	for _, group := range locoFunctionGroups {
		m.HandleLANXDB0(xHeaderSetLocoDrive, byte(group), s.locoFunctionGroupHandler())
	}
	m.HandleLANXDB0(xHeaderSetLocoBinary, xCommandSetLocoBinary, locoBinaryStateHandler())
	m.HandleLANX(xHeaderSetLocoEStop, s.locoEStopHandler())

	for _, header := range []uint16{
		protocol.HeaderLANCANDeviceGetDescription,
		protocol.HeaderLANCANDeviceSetDescription,
		protocol.HeaderLANCANDetector,
		protocol.HeaderLANCANMaintenance,
		protocol.HeaderLANCANBoosterSetTrackPower,
	} {
		m.Handle(header, server.HandlerFunc(s.serveCAN))
	}
	return m
}

// reply adapts a function building a single reply to a Handler.
func reply(build func() protocol.Message) server.Handler {
	return server.HandlerFunc(func(w server.ResponseWriter, _ *server.Request) {
		w.Reply(build())
	})
}

// Identity returns what the simulator reports about itself.
func (s *Server) Identity() Identity {
	return s.identity
}

func (s *Server) hwInfoReply() protocol.Message {
	return protocol.EncodeHWInfo(protocol.HWInfo{HwType: s.identity.HwType, FirmwareVersion: s.identity.Firmware})
}
