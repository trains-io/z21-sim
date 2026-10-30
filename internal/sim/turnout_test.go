package sim_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trains-io/z21-sim/internal/sim"
	"github.com/trains-io/z21.go/client"
	"github.com/trains-io/z21.go/protocol"
)

// readLANX waits for a LAN_X dataset that parse accepts and returns it parsed.
func readLANX[T any](t *testing.T, c *client.Client, what string, parse func([]byte) (T, error)) T {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	for {
		msgs, err := c.ReadPacket(ctx)
		if err != nil {
			t.Fatalf("no %s: %v", what, err)
		}
		for _, msg := range msgs {
			if msg.Header != protocol.HeaderLANX {
				continue
			}
			if v, err := parse(msg.Data); err == nil {
				return v
			}
		}
	}
}

func readTurnoutInfo(t *testing.T, c *client.Client) protocol.TurnoutInfo {
	t.Helper()
	return readLANX(t, c, "LAN_X_TURNOUT_INFO", protocol.ParseTurnoutInfo)
}

func readExtAccessoryInfo(t *testing.T, c *client.Client) protocol.ExtAccessoryInfo {
	t.Helper()
	return readLANX(t, c, "LAN_X_EXT_ACCESSORY_INFO", protocol.ParseExtAccessoryInfo)
}

// send sends req from c.
func send(t *testing.T, c *client.Client, req protocol.Message) {
	t.Helper()
	if err := c.Send(t.Context(), req); err != nil {
		t.Fatalf("Send(% x): %v", req.Data, err)
	}
}

// requireOnlyXStatus sends reqs followed by LAN_X_GET_STATUS from c and checks
// that the next packet c receives holds the status only: nothing for reqs and
// no broadcast before it.
func requireOnlyXStatus(t *testing.T, c *client.Client, reqs ...protocol.Message) {
	t.Helper()
	if err := c.SendMany(t.Context(), append(reqs, protocol.GetXStatus())...); err != nil {
		t.Fatalf("SendMany: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	msgs, err := c.ReadPacket(ctx)
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("replies = %+v, want only the status", msgs)
	}
	if _, err := protocol.ParseXStatus(msgs[0].Data); err != nil {
		t.Fatalf("reply = %+v, want LAN_X_STATUS_CHANGED: %v", msgs[0], err)
	}
}

func dialUnsubscribed(t *testing.T, addr string) *client.Client {
	t.Helper()
	c, err := client.Dial(addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestGetTurnoutInfo(t *testing.T) {
	_, addr := startTestServer(t)
	c := dialXBus(t, addr)

	msgs, err := c.Call(t.Context(), protocol.GetTurnoutInfo(7))
	if err != nil {
		t.Fatalf("Call(GetTurnoutInfo(7)): %v", err)
	}
	got, err := protocol.TurnoutInfoFromMessages(msgs)
	want := protocol.TurnoutInfo{Address: 7, Position: protocol.TurnoutPositionNotSwitched}
	if err != nil || got != want {
		t.Fatalf("TurnoutInfo = %+v, err=%v; want %+v", got, err, want)
	}
}

func TestSetTurnoutNotifiesSubscribers(t *testing.T) {
	_, addr := startTestServer(t)
	observer := dialXBus(t, addr)
	switcher := dialXBus(t, addr)
	want := protocol.TurnoutInfo{Address: 7, Position: protocol.TurnoutPositionOutput2}

	// Activate, then deactivate, as throttles do: both are confirmed.
	for _, sw := range []protocol.TurnoutSwitch{{Activate: true, Output2: true}, {Output2: true}} {
		send(t, switcher, protocol.SetTurnout(7, sw))
		for name, c := range map[string]*client.Client{"observer": observer, "switcher": switcher} {
			if got := readTurnoutInfo(t, c); got != want {
				t.Fatalf("%+v: %s got %+v, want %+v", sw, name, got, want)
			}
		}
	}

	msgs, err := observer.Call(t.Context(), protocol.GetTurnoutInfo(7))
	if err != nil {
		t.Fatalf("Call(GetTurnoutInfo(7)): %v", err)
	}
	if got, err := protocol.TurnoutInfoFromMessages(msgs); err != nil || got != want {
		t.Fatalf("TurnoutInfo = %+v, err=%v; want %+v", got, err, want)
	}
}

func TestSetTurnoutUnsubscribedRequester(t *testing.T) {
	_, addr := startTestServer(t)
	observer := dialXBus(t, addr)
	switcher := dialUnsubscribed(t, addr)

	requireOnlyXStatus(t, switcher, protocol.SetTurnout(7, protocol.TurnoutSwitch{Activate: true}))
	want := protocol.TurnoutInfo{Address: 7, Position: protocol.TurnoutPositionOutput1}
	if got := readTurnoutInfo(t, observer); got != want {
		t.Fatalf("observer got %+v, want %+v", got, want)
	}
}

func TestGetExtAccessoryInfo(t *testing.T) {
	_, addr := startTestServer(t)
	c := dialXBus(t, addr)

	send(t, c, protocol.GetExtAccessoryInfo(7))
	want := protocol.ExtAccessoryInfo{Address: 7, Status: protocol.ExtAccessoryStatusUnknown}
	if got := readExtAccessoryInfo(t, c); got != want {
		t.Fatalf("ExtAccessoryInfo = %+v, want %+v", got, want)
	}
}

func TestSetExtAccessoryNotifiesSubscribers(t *testing.T) {
	_, addr := startTestServer(t)
	observer := dialXBus(t, addr)
	switcher := dialXBus(t, addr)
	want := protocol.ExtAccessoryInfo{Address: 7, Value: 0x12, Status: protocol.ExtAccessoryStatusValid}

	for range 2 {
		send(t, switcher, protocol.SetExtAccessory(7, 0x12))
		for name, c := range map[string]*client.Client{"observer": observer, "switcher": switcher} {
			if got := readExtAccessoryInfo(t, c); got != want {
				t.Fatalf("%s got %+v, want %+v", name, got, want)
			}
		}
	}

	send(t, observer, protocol.GetExtAccessoryInfo(7))
	if got := readExtAccessoryInfo(t, observer); got != want {
		t.Fatalf("ExtAccessoryInfo = %+v, want %+v", got, want)
	}
}

func TestSetExtAccessoryUnsubscribedRequester(t *testing.T) {
	_, addr := startTestServer(t)
	observer := dialXBus(t, addr)
	switcher := dialUnsubscribed(t, addr)

	requireOnlyXStatus(t, switcher, protocol.SetExtAccessory(7, 0x12))
	want := protocol.ExtAccessoryInfo{Address: 7, Value: 0x12, Status: protocol.ExtAccessoryStatusValid}
	if got := readExtAccessoryInfo(t, observer); got != want {
		t.Fatalf("observer got %+v, want %+v", got, want)
	}
}

func TestAccessoryRequestsInvalid(t *testing.T) {
	const tooHigh = sim.MaxAccessoryAddress + 1
	tests := []struct {
		name string
		req  protocol.Message
	}{
		{name: "get turnout info address too high", req: protocol.GetTurnoutInfo(tooHigh)},
		{name: "set turnout address too high", req: protocol.SetTurnout(tooHigh, protocol.TurnoutSwitch{Activate: true})},
		{name: "set turnout too short", req: protocol.EncodeLANX(0x53, 0x00, 0x07)},
		{name: "get ext accessory info address too high", req: protocol.GetExtAccessoryInfo(tooHigh)},
		{name: "set ext accessory address too high", req: protocol.SetExtAccessory(tooHigh, 1)},
		{name: "set ext accessory too short", req: protocol.EncodeLANX(0x54, 0x00, 0x07, 0x01)},
	}
	_, addr := startTestServer(t)
	observer := dialXBus(t, addr)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := dialXBus(t, addr)
			send(t, c, tt.req)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			msgs, err := c.ReadPacket(ctx)
			if err != nil {
				t.Fatalf("ReadPacket: %v", err)
			}
			if len(msgs) != 1 || !protocol.IsUnknownCommand(msgs[0].Data) {
				t.Fatalf("reply = %+v, want LAN_X_UNKNOWN_COMMAND", msgs)
			}
		})
	}

	// None of the invalid requests was broadcast: the observer's next packet
	// is its own status reply.
	requireOnlyXStatus(t, observer)
}

func TestServerSetTurnout(t *testing.T) {
	srv, addr := startTestServer(t)
	observer := dialXBus(t, addr)

	info, recipients, err := srv.SetTurnout(7, protocol.TurnoutPositionOutput2)
	want := protocol.TurnoutInfo{Address: 7, Position: protocol.TurnoutPositionOutput2}
	if err != nil || info != want || recipients != 1 {
		t.Fatalf("SetTurnout = %+v, %d, %v; want %+v, 1 recipient", info, recipients, err, want)
	}
	if got := readTurnoutInfo(t, observer); got != want {
		t.Fatalf("observer got %+v, want %+v", got, want)
	}

	for _, pos := range []protocol.TurnoutPosition{protocol.TurnoutPositionNotSwitched, protocol.TurnoutPositionInvalid} {
		if _, _, err := srv.SetTurnout(7, pos); !errors.Is(err, sim.ErrInvalidAccessory) {
			t.Fatalf("SetTurnout(7, %#x) error = %v, want ErrInvalidAccessory", pos, err)
		}
	}
	if _, _, err := srv.SetTurnout(sim.MaxAccessoryAddress+1, protocol.TurnoutPositionOutput1); !errors.Is(err, sim.ErrInvalidAccessory) {
		t.Fatalf("SetTurnout(%d) error = %v, want ErrInvalidAccessory", sim.MaxAccessoryAddress+1, err)
	}
}

func TestServerSetExtAccessory(t *testing.T) {
	srv, addr := startTestServer(t)
	observer := dialXBus(t, addr)

	info, recipients, err := srv.SetExtAccessory(7, 0x12)
	want := protocol.ExtAccessoryInfo{Address: 7, Value: 0x12, Status: protocol.ExtAccessoryStatusValid}
	if err != nil || info != want || recipients != 1 {
		t.Fatalf("SetExtAccessory = %+v, %d, %v; want %+v, 1 recipient", info, recipients, err, want)
	}
	if got := readExtAccessoryInfo(t, observer); got != want {
		t.Fatalf("observer got %+v, want %+v", got, want)
	}

	if _, _, err := srv.SetExtAccessory(sim.MaxAccessoryAddress+1, 1); !errors.Is(err, sim.ErrInvalidAccessory) {
		t.Fatalf("SetExtAccessory(%d) error = %v, want ErrInvalidAccessory", sim.MaxAccessoryAddress+1, err)
	}
}
