package sim

import "io"

// Option configures a Server.
type Option func(*Server)

// WithTrace enables LAN traffic dumps to out.
func WithTrace(out io.Writer) Option {
	return func(s *Server) {
		s.traceOut = out
	}
}
