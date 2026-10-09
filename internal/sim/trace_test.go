package sim_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trains-io/z21-sim/internal/sim"
	"github.com/trains-io/z21.go/client"
	"github.com/trains-io/z21.go/protocol"
)

func TestTraceLAN(t *testing.T) {
	var buf bytes.Buffer

	srv, err := sim.New("127.0.0.1:0", nil, sim.WithTrace(&buf))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- srv.Serve(ctx)
	}()
	stop := sync.OnceFunc(func() {
		cancel()
		<-done
		_ = srv.Close()
	})
	t.Cleanup(stop)

	c, err := client.Dial(srv.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	reqCtx, reqCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer reqCancel()

	if _, err := c.Call(reqCtx, protocol.GetHWInfo()); err != nil {
		t.Fatalf("Call: %v", err)
	}

	// The server goroutine writes buf; stop it before reading.
	stop()
	out := buf.String()
	if !strings.Contains(out, "LAN <<") {
		t.Fatalf("missing RX trace: %q", out)
	}
	if !strings.Contains(out, "LAN >>") {
		t.Fatalf("missing TX trace: %q", out)
	}
	if !strings.Contains(out, "LAN_GET_HWINFO") {
		t.Fatalf("missing message name: %q", out)
	}
}

func TestTraceCANBroadcastWithoutSubscribers(t *testing.T) {
	var buf bytes.Buffer

	srv, err := sim.New("127.0.0.1:0", nil, sim.WithTrace(&buf))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.Close()

	device, err := sim.BuildCANDevice(0xDB04, sim.CANDeviceDetector, "Block 4", 60, 2, 0)
	if err != nil {
		t.Fatalf("BuildCANDevice: %v", err)
	}

	if count := srv.NotifyCANDeviceDetected(device); count != 0 {
		t.Fatalf("RecipientCount = %d, want 0", count)
	}

	out := buf.String()
	if !strings.Contains(out, "LAN >> broadcast CAN detector (0 recipients)") {
		t.Fatalf("missing broadcast trace: %q", out)
	}
	if !strings.Contains(out, "LAN_CAN_DETECTOR") {
		t.Fatalf("missing detector message: %q", out)
	}
}
