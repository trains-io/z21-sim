package trace_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/trains-io/z21-sim/internal/trace"
)

func TestLogGRPC(t *testing.T) {
	var buf bytes.Buffer
	tr := trace.New(&buf)

	tr.LogGRPC("/trains.io.z21.simctl.v1.SimulatorService/SetTrackPower", nil)

	out := buf.String()
	if !strings.Contains(out, "gRPC << /trains.io.z21.simctl.v1.SimulatorService/SetTrackPower") {
		t.Fatalf("output = %q", out)
	}
}

func TestNewNil(t *testing.T) {
	if tr := trace.New(nil); tr != nil {
		t.Fatalf("New(nil) = %v, want nil", tr)
	}
	var tr *trace.Tracer
	tr.LogGRPC("/x", nil) // must not panic
}
