// Package z21simctl implements the commands of z21-sim-ctl, the tool that
// changes a running z21-sim through its control API.
package z21simctl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/trains-io/z21-sim/simclient"
)

// EnvAddr overrides the default of --addr.
const EnvAddr = "Z21_SIM_CTL_ADDR"

const (
	outputText = "text"
	outputJSON = "json"
)

type app struct {
	addr    string
	timeout time.Duration
	output  string
}

// NewRootCmd returns the z21-sim-ctl root command.
func NewRootCmd() *cobra.Command {
	a := &app{}
	root := &cobra.Command{
		Use:           "z21-sim-ctl",
		Short:         "Change a running z21-sim at run time",
		Long:          "z21-sim-ctl changes the state of a running z21-sim through its gRPC control API.\nZ21 LAN clients subscribed to the matching broadcasts see the changes.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			if a.output != outputText && a.output != outputJSON {
				return fmt.Errorf("invalid --output %q: want %s or %s", a.output, outputText, outputJSON)
			}
			return nil
		},
	}

	addr := simclient.DefaultAddr
	if env := os.Getenv(EnvAddr); env != "" {
		addr = env
	}
	flags := root.PersistentFlags()
	flags.StringVar(&a.addr, "addr", addr, "control API address of z21-sim (env "+EnvAddr+")")
	flags.DurationVar(&a.timeout, "timeout", 5*time.Second, "timeout per request")
	flags.StringVarP(&a.output, "output", "o", outputText, "output format: text or json")

	root.AddCommand(
		newPowerCmd(a),
		newStopCmd(a),
		newSystemCmd(a),
		newClientsCmd(a),
	)
	return root
}

// withClient runs fn with a control API client and a context bounded by
// --timeout.
func (a *app) withClient(cmd *cobra.Command, fn func(context.Context, *simclient.Client) error) error {
	c, err := simclient.Dial(a.addr)
	if err != nil {
		return err
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), a.timeout)
	defer cancel()
	return fn(ctx, c)
}

// print writes v as JSON with --output json, otherwise calls text.
func (a *app) print(cmd *cobra.Command, v any, text func(io.Writer)) error {
	w := cmd.OutOrStdout()
	if a.output == outputJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	text(w)
	return nil
}
