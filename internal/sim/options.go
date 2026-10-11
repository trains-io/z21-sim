package sim

import "io"

// Option configures a Server.
type Option func(*Server)

// WithAdditionalAddr also binds a UDP socket on addr (for example
// "0.0.0.0:21106"), sharing client sessions with the main socket.
func WithAdditionalAddr(addr string) Option {
	return func(s *Server) {
		s.additionalAddrs = append(s.additionalAddrs, addr)
	}
}

// WithTrace enables LAN traffic dumps to out.
func WithTrace(out io.Writer) Option {
	return func(s *Server) {
		s.traceOut = out
	}
}
