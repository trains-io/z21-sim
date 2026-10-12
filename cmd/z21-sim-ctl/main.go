package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/trains-io/z21-sim/internal/cli/z21simctl"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := z21simctl.NewRootCmd().ExecuteContext(ctx)
	stop()
	if err != nil {
		fmt.Fprintf(os.Stderr, "z21-sim-ctl: %v\n", err)
		os.Exit(1)
	}
}
