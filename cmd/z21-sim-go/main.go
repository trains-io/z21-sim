package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/trains-io/z21-sim-go/internal/server"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:21105", "UDP listen address")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	srv, err := server.New(*addr, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "z21-sim-go: %v\n", err)
		os.Exit(1)
	}
	defer srv.Close()

	log.Info("socket bound", "addr", srv.Addr())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Serve(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "z21-sim-go: %v\n", err)
		os.Exit(1)
	}
}
