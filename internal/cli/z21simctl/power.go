package z21simctl

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/trains-io/z21-sim/simclient"
)

type powerResult struct {
	On      bool `json:"on"`
	Changed bool `json:"changed"`
}

func newPowerCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:       "power on|off",
		Short:     "Switch track power on or off",
		Long:      "Switch track power on or off. X-Bus subscribers get LAN_X_BC_TRACK_POWER_ON/OFF,\nsystem-state subscribers get LAN_SYSTEMSTATE_DATACHANGED. \"power on\" also ends\nan emergency stop and a short circuit.",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"on", "off"},
		RunE: func(cmd *cobra.Command, args []string) error {
			on := args[0] == "on"
			return a.withClient(cmd, func(ctx context.Context, c *simclient.Client) error {
				changed, err := c.SetTrackPower(ctx, on)
				if err != nil {
					return err
				}
				return a.print(cmd, powerResult{On: on, Changed: changed}, func(w io.Writer) {
					fmt.Fprintf(w, "track power %s%s\n", args[0], unchangedSuffix(changed))
				})
			})
		},
	}
}

func unchangedSuffix(changed bool) string {
	if changed {
		return ""
	}
	return " (unchanged)"
}
