package grpcserver

import (
	"context"
	"io"

	"github.com/trains-io/z21-sim/internal/trace"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// Option configures a Server.
type Option func(*Server)

// WithTrace enables gRPC request dumps to out.
func WithTrace(out io.Writer) Option {
	return func(g *Server) {
		g.tracer = trace.New(out)
	}
}

func grpcTraceInterceptor(tr *trace.Tracer) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if tr != nil {
			if msg, ok := req.(proto.Message); ok {
				tr.LogGRPC(info.FullMethod, msg)
			} else {
				tr.LogGRPC(info.FullMethod, nil)
			}
		}
		return handler(ctx, req)
	}
}
