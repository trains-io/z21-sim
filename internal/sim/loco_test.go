package sim_test

import (
	"context"
	"testing"
	"time"

	"github.com/trains-io/z21.go/client"
	"github.com/trains-io/z21.go/protocol"
)

// dialXBus dials addr as a client subscribed to X-Bus broadcasts.
func dialXBus(t *testing.T, addr string) *client.Client {
	t.Helper()
	c, err := client.Dial(addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	if err := c.Send(t.Context(), protocol.SetBroadcastFlags(protocol.BroadcastFlagXpressNet)); err != nil {
		t.Fatalf("SetBroadcastFlags: %v", err)
	}
	if _, err := c.Call(t.Context(), protocol.GetBroadcastFlags()); err != nil {
		t.Fatalf("GetBroadcastFlags: %v", err)
	}
	return c
}

// getLocoInfo asks for the state of addr, which also subscribes c to it.
func getLocoInfo(t *testing.T, c *client.Client, addr uint16) protocol.LocoInfo {
	t.Helper()
	msgs, err := c.Call(t.Context(), protocol.GetLocoInfo(addr))
	if err != nil {
		t.Fatalf("Call(GetLocoInfo(%d)): %v", addr, err)
	}
	info, err := protocol.LocoInfoFromMessages(msgs)
	if err != nil {
		t.Fatalf("LocoInfoFromMessages: %v", err)
	}
	return info
}

// readLocoInfo waits for an unsolicited LAN_X_LOCO_INFO for addr.
func readLocoInfo(t *testing.T, c *client.Client, addr uint16) protocol.LocoInfo {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	for {
		msgs, err := c.ReadPacket(ctx)
		if err != nil {
			t.Fatalf("no LAN_X_LOCO_INFO for loco %d: %v", addr, err)
		}
		for _, msg := range msgs {
			if msg.Header != protocol.HeaderLANX {
				continue
			}
			if info, err := protocol.ParseLocoInfo(msg.Data); err == nil && info.Address == addr {
				return info
			}
		}
	}
}

func TestGetLocoInfo(t *testing.T) {
	_, addr := startTestServer(t)
	c := dialXBus(t, addr)

	got := getLocoInfo(t, c, 3)
	want := protocol.LocoInfo{Address: 3, SpeedSteps: 0x04, Forward: true}
	if got != want {
		t.Fatalf("LocoInfo = %+v, want %+v (default loco)", got, want)
	}
}

func TestDriveLocoNotifiesSubscribers(t *testing.T) {
	_, addr := startTestServer(t)
	observer := dialXBus(t, addr)
	driver := dialXBus(t, addr)
	getLocoInfo(t, observer, 3)

	if err := driver.Send(t.Context(), protocol.SetLocoDrive(3, protocol.LocoSpeedSteps128, false, 40)); err != nil {
		t.Fatalf("SetLocoDrive: %v", err)
	}
	want := protocol.LocoInfo{Address: 3, SpeedSteps: 0x04, Speed: 40}
	if got := readLocoInfo(t, observer, 3); got != want {
		t.Fatalf("observer got %+v, want %+v", got, want)
	}

	// Subscribed now, the driver gets the loco info of its own drive too.
	if got := getLocoInfo(t, driver, 3); got != want {
		t.Fatalf("driver GetLocoInfo = %+v, want %+v", got, want)
	}
	if err := driver.Send(t.Context(), protocol.SetLocoDrive(3, protocol.LocoSpeedSteps128, true, 60)); err != nil {
		t.Fatalf("SetLocoDrive: %v", err)
	}
	want = protocol.LocoInfo{Address: 3, SpeedSteps: 0x04, Forward: true, Speed: 60}
	for name, c := range map[string]*client.Client{"observer": observer, "driver": driver} {
		if got := readLocoInfo(t, c, 3); got != want {
			t.Fatalf("%s got %+v, want %+v", name, got, want)
		}
	}
}

func TestDriveLocoMalformed(t *testing.T) {
	_, addr := startTestServer(t)
	c := dialXBus(t, addr)

	tooShort := protocol.EncodeLANX(0xE4, 0x13, 0x00, 0x03)
	if err := c.Send(t.Context(), tooShort); err != nil {
		t.Fatalf("Send: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	msgs, err := c.ReadPacket(ctx)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	if len(msgs) != 1 || !protocol.IsUnknownCommand(msgs[0].Data) {
		t.Fatalf("reply = %+v, want LAN_X_UNKNOWN_COMMAND", msgs)
	}
}
