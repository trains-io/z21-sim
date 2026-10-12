package z21simctl

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/trains-io/z21-sim/simclient"
)

type shortCircuitResult struct {
	Kind    string `json:"kind"`
	Changed bool   `json:"changed"`
}

func newShortCircuitCmd(a *app) *cobra.Command {
	var external bool
	cmd := &cobra.Command{
		Use:   "short-circuit",
		Short: "Simulate a short circuit on the track",
		Long: "Simulate a short circuit: the track voltage goes off. X-Bus subscribers get\n" +
			"LAN_X_BC_TRACK_SHORT_CIRCUIT and LAN_X_STATUS_CHANGED, system-state\n" +
			"subscribers LAN_SYSTEMSTATE_DATACHANGED, which tells internal shorts (the\n" +
			"Z21's own track output) and external ones (a booster) apart.\n\n" +
			"\"z21-sim-ctl power on\" ends the short circuit.",
		Example: "  z21-sim-ctl short-circuit\n" +
			"  z21-sim-ctl short-circuit --external",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			kind, name := simclient.ShortCircuitInternal, "internal"
			if external {
				kind, name = simclient.ShortCircuitExternal, "external"
			}
			return a.withClient(cmd, func(ctx context.Context, c *simclient.Client) error {
				changed, err := c.ShortCircuit(ctx, kind)
				if err != nil {
					return err
				}
				return a.print(cmd, shortCircuitResult{Kind: name, Changed: changed}, func(w io.Writer) {
					fmt.Fprintf(w, "short circuit (%s)%s\n", name, unchangedSuffix(changed))
				})
			})
		},
	}
	cmd.Flags().BoolVar(&external, "external", false, "short on an external booster output instead of the Z21's own")
	return cmd
}
