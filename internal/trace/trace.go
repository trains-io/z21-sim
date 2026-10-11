// Package trace dumps gRPC control traffic. Z21 LAN traffic is traced by
// z21.go/server (server.NewTracer).
package trace

import (
	"fmt"
	"io"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Tracer writes human-readable gRPC request dumps.
type Tracer struct {
	mu  sync.Mutex
	out io.Writer
}

// New returns a tracer writing to out, or nil when out is nil.
func New(out io.Writer) *Tracer {
	if out == nil {
		return nil
	}
	return &Tracer{out: out}
}

// LogGRPC dumps an incoming gRPC request.
func (t *Tracer) LogGRPC(method string, req proto.Message) {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	fmt.Fprintf(t.out, "gRPC << %s\n", method)
	if req == nil {
		return
	}
	if b, err := protojson.Marshal(req); err == nil {
		fmt.Fprintf(t.out, "   %s\n", string(b))
		return
	}
	fmt.Fprintf(t.out, "   %v\n", req)
}
