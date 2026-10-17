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
	"github.com/spf13/pflag"
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
	_ = root.RegisterFlagCompletionFunc("output", cobra.FixedCompletions(
		[]string{outputText, outputJSON}, cobra.ShellCompDirectiveNoFileComp))

	root.AddCommand(
		newPowerCmd(a),
		newStopCmd(a),
		newShortCircuitCmd(a),
		newSystemCmd(a),
		newCANCmd(a),
		newClientsCmd(a),
	)
	disableFileCompletion(root)
	return root
}

// disableFileCompletion stops shells from offering file names for arguments
// and flag values of cmd and its subcommands that have no completion of their
// own: no command takes a file.
func disableFileCompletion(cmd *cobra.Command) {
	if cmd.ValidArgsFunction == nil && len(cmd.ValidArgs) == 0 {
		cmd.ValidArgsFunction = cobra.NoFileCompletions
	}
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.NoOptDefVal != "" {
			return
		}
		if _, ok := cmd.GetFlagCompletionFunc(f.Name); !ok {
			_ = cmd.RegisterFlagCompletionFunc(f.Name, cobra.NoFileCompletions)
		}
	})
	for _, sub := range cmd.Commands() {
		disableFileCompletion(sub)
	}
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
