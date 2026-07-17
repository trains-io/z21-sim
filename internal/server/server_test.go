package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/trains-io/z21-sim-go/internal/server"
	"github.com/trains-io/z21.go/client"
	"github.com/trains-io/z21.go/protocol"
)

func startTestServer(t *testing.T) (addr string, stop func()) {
	t.Helper()

	srv, err := server.New("127.0.0.1:0", nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- srv.Serve(ctx)
	}()

	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Log("server did not stop cleanly")
		}
		_ = srv.Close()
	})

	return srv.Addr().String(), func() { cancel() }
}

func TestGetHWInfo(t *testing.T) {
	addr, _ := startTestServer(t)

	c, err := client.Dial(addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msgs, err := c.Call(ctx, protocol.GetHWInfo())
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	hw, err := protocol.HWInfoFromMessages(msgs)
	if err != nil {
		t.Fatalf("HWInfoFromMessages: %v", err)
	}
	if hw.HwType != protocol.HwTypeZ21New {
		t.Fatalf("HwType = %#x, want %#x", hw.HwType, protocol.HwTypeZ21New)
	}
}

func TestGetSerialNumber(t *testing.T) {
	addr, _ := startTestServer(t)

	c, err := client.Dial(addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msgs, err := c.Call(ctx, protocol.GetSerialNumber())
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	serial, ok := protocol.SerialFromMessages(msgs)
	if !ok {
		t.Fatalf("SerialFromMessages() ok = false, msgs = %#v", msgs)
	}
	if len(serial) < 4 || serial[:3] != "999" {
		t.Fatalf("serial = %q, want 999-prefix simulator serial", serial)
	}
}

func TestGetCode(t *testing.T) {
	addr, _ := startTestServer(t)

	c, err := client.Dial(addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msgs, err := c.Call(ctx, protocol.GetCode())
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	code, err := protocol.CodeFromMessages(msgs)
	if err != nil {
		t.Fatalf("CodeFromMessages: %v", err)
	}
	if code != protocol.CodeNoLock {
		t.Fatalf("code = %#x, want %#x", code, protocol.CodeNoLock)
	}
}
