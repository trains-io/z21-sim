package z21simctl

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/trains-io/z21-sim/simclient"
)

func newSystemCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "Change or send the system state",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newSystemSetCmd(a), newSystemBroadcastCmd(a))
	return cmd
}

type systemSetResult struct {
	Changed    bool `json:"changed"`
	Recipients int  `json:"recipients"`
}

func newSystemSetCmd(a *app) *cobra.Command {
	var (
		mainCurrent, progCurrent, filteredCurrent, temperature int16
		supplyVoltage, vccVoltage                              uint16
	)
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Change measured system-state values",
		Long: "Change the currents, voltages and temperature the simulator reports in\n" +
			"LAN_SYSTEMSTATE_DATACHANGED. Values that are not given stay unchanged; if\n" +
			"anything changes, system-state subscribers get the new state.",
		Example: "  z21-sim-ctl system set --main-current 1200 --temperature 55\n" +
			"  z21-sim-ctl system set --supply-voltage 18000 --vcc-voltage 16500",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags := cmd.Flags()
			var v simclient.SystemValues
			if flags.Changed("main-current") {
				v.MainCurrent = &mainCurrent
			}
			if flags.Changed("prog-current") {
				v.ProgCurrent = &progCurrent
			}
			if flags.Changed("filtered-main-current") {
				v.FilteredMainCurrent = &filteredCurrent
			}
			if flags.Changed("temperature") {
				v.Temperature = &temperature
			}
			if flags.Changed("supply-voltage") {
				v.SupplyVoltage = &supplyVoltage
			}
			if flags.Changed("vcc-voltage") {
				v.VCCVoltage = &vccVoltage
			}
			if v == (simclient.SystemValues{}) {
				return errors.New("no values given; set at least one of --main-current, --prog-current, --filtered-main-current, --temperature, --supply-voltage, --vcc-voltage")
			}

			return a.withClient(cmd, func(ctx context.Context, c *simclient.Client) error {
				changed, recipients, err := c.SetSystemState(ctx, v)
				if err != nil {
					return err
				}
				return a.print(cmd, systemSetResult{Changed: changed, Recipients: recipients}, func(w io.Writer) {
					if !changed {
						fmt.Fprintln(w, "system state unchanged")
						return
					}
					fmt.Fprintf(w, "system state changed, sent to %s\n", plural(recipients, "client"))
				})
			})
		},
	}
	flags := cmd.Flags()
	flags.Int16Var(&mainCurrent, "main-current", 0, "current on the main track, mA")
	flags.Int16Var(&progCurrent, "prog-current", 0, "current on the programming track, mA")
	flags.Int16Var(&filteredCurrent, "filtered-main-current", 0, "smoothed current on the main track, mA")
	flags.Int16Var(&temperature, "temperature", 0, "internal temperature, °C")
	flags.Uint16Var(&supplyVoltage, "supply-voltage", 0, "supply voltage, mV")
	flags.Uint16Var(&vccVoltage, "vcc-voltage", 0, "internal voltage (= track voltage), mV")
	return cmd
}

type recipientsResult struct {
	Recipients int `json:"recipients"`
}

func newSystemBroadcastCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "broadcast",
		Short: "Send the current system state to system-state subscribers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withClient(cmd, func(ctx context.Context, c *simclient.Client) error {
				recipients, err := c.BroadcastSystemState(ctx)
				if err != nil {
					return err
				}
				return a.print(cmd, recipientsResult{Recipients: recipients}, func(w io.Writer) {
					fmt.Fprintf(w, "system state sent to %s\n", plural(recipients, "client"))
				})
			})
		},
	}
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
