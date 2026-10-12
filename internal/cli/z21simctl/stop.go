package z21simctl

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/trains-io/z21-sim/simclient"
)

type stopResult struct {
	Changed bool `json:"changed"`
}

func newStopCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Trigger an emergency stop",
		Long: "Trigger an emergency stop, as LAN_X_SET_STOP does: all locos stop, the track\n" +
			"voltage stays as it is. X-Bus subscribers get LAN_X_BC_STOPPED and\n" +
			"LAN_X_STATUS_CHANGED, system-state subscribers LAN_SYSTEMSTATE_DATACHANGED.\n\n" +
			"\"z21-sim-ctl power on\" ends the emergency stop.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withClient(cmd, func(ctx context.Context, c *simclient.Client) error {
				changed, err := c.EmergencyStop(ctx)
				if err != nil {
					return err
				}
				return a.print(cmd, stopResult{Changed: changed}, func(w io.Writer) {
					fmt.Fprintf(w, "emergency stop%s\n", unchangedSuffix(changed))
				})
			})
		},
	}
}
