package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/trains-io/z21-sim/internal/sim"
	"github.com/trains-io/z21-sim/internal/sim/grpcserver"
	"github.com/trains-io/z21.go/protocol"
)

func main() {
	udpAddr := flag.String("addr", "0.0.0.0:21105", "UDP listen address")
	altAddr := flag.String("alt-addr", "0.0.0.0:21106", "second UDP listen address sharing client sessions (empty disables)")
	grpcAddr := flag.String("grpc-addr", "127.0.0.1:50051", "gRPC control listen address (empty disables)")
	traceFlag := flag.Bool("trace", false, "dump LAN and gRPC traffic to stderr")
	id := sim.DefaultIdentity()
	identityFlags(&id)
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	log.Info("identity",
		"serial", id.Serial,
		"hw_type", protocol.FormatHwType(id.HwType),
		"firmware", protocol.FormatFirmwareVersion(id.Firmware),
		"code", protocol.FormatLockCode(id.Code),
		"capabilities", fmt.Sprintf("%#02x", id.Capabilities),
	)

	simOpts := []sim.Option{sim.WithIdentity(id)}
	var grpcTraceOpts []grpcserver.Option
	if *altAddr != "" {
		simOpts = append(simOpts, sim.WithAdditionalAddr(*altAddr))
	}
	if *traceFlag {
		simOpts = append(simOpts, sim.WithTrace(os.Stderr))
		grpcTraceOpts = append(grpcTraceOpts, grpcserver.WithTrace(os.Stderr))
	}

	srv, err := sim.New(*udpAddr, log, simOpts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "z21-sim: %v\n", err)
		os.Exit(1)
	}
	defer srv.Close()

	for _, addr := range srv.Addrs() {
		log.Info("socket bound", "addr", addr)
	}

	var grpcSrv *grpcserver.Server
	if *grpcAddr != "" {
		grpcSrv, err = grpcserver.New(srv, *grpcAddr, log, grpcTraceOpts...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "z21-sim: %v\n", err)
			os.Exit(1)
		}
		defer grpcSrv.Stop()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	if grpcSrv != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := grpcSrv.Serve(); err != nil && !errors.Is(err, net.ErrClosed) {
				errCh <- fmt.Errorf("gRPC: %w", err)
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.Serve(ctx); err != nil {
			errCh <- fmt.Errorf("UDP: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		if grpcSrv != nil {
			grpcSrv.Stop()
		}
	case err := <-errCh:
		fmt.Fprintf(os.Stderr, "z21-sim: %v\n", err)
		os.Exit(1)
	}

	wg.Wait()
}

// identityFlags registers the flags that override parts of id.
func identityFlags(id *sim.Identity) {
	flag.Func("serial", "serial number (default 999 followed by six digits from the start time)", func(s string) error {
		n, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			return fmt.Errorf("want a decimal number up to %d", uint32(math.MaxUint32))
		}
		id.Serial = uint32(n)
		return nil
	})
	flag.Func("hw-type", "hardware type: "+strings.Join(sim.HwTypeNames(), ", ")+", or a number such as 0x0211 (default z21)", func(s string) (err error) {
		id.HwType, err = sim.ParseHwType(s)
		return err
	})
	flag.Func("firmware", "firmware version major.minor, also reported to X-Bus clients (default 1.20)", func(s string) (err error) {
		id.Firmware, err = sim.ParseFirmwareVersion(s)
		return err
	})
	flag.Func("code", "lock code: none, start-locked, start-unlocked (default none)", func(s string) (err error) {
		id.Code, err = sim.ParseCode(s)
		return err
	})
	flag.Func("capabilities", "capabilities byte reported in the system state, decimal or 0x hex (default 0x31: DCC, loco and accessory commands)", func(s string) error {
		n, err := strconv.ParseUint(s, 0, 8)
		if err != nil {
			return errors.New("want a number from 0 to 255 (0xff)")
		}
		id.Capabilities = byte(n)
		return nil
	})
}
