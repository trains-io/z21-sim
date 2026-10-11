package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/trains-io/z21-sim/internal/sim"
	"github.com/trains-io/z21-sim/internal/sim/grpcserver"
)

func main() {
	udpAddr := flag.String("addr", "0.0.0.0:21105", "UDP listen address")
	altAddr := flag.String("alt-addr", "0.0.0.0:21106", "second UDP listen address sharing client sessions (empty disables)")
	grpcAddr := flag.String("grpc-addr", "127.0.0.1:50051", "gRPC control listen address (empty disables)")
	traceFlag := flag.Bool("trace", false, "dump LAN and gRPC traffic to stderr")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	var simOpts []sim.Option
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
