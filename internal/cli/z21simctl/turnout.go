package z21simctl

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/trains-io/z21-sim/simclient"
)

const maxAccessoryAddress = 2047

func newTurnoutCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "turnout",
		Short: "Switch turnouts as a throttle outside the Z21 LAN",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newTurnoutSetCmd(a))
	return cmd
}

type turnoutSetResult struct {
	Address    uint16 `json:"address"`
	Output     int    `json:"output"`
	Recipients int    `json:"recipients"`
}

func newTurnoutSetCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "set ADDR 1|2",
		Short: "Switch a turnout to output 1 or 2",
		Long: "Switch a turnout as a throttle outside the Z21 LAN would. X-Bus subscribers get\n" +
			"LAN_X_TURNOUT_INFO, also when the turnout is already in that position.\n\n" +
			"ADDR is the address as on the wire (FAdr, 0 to 2047): 0 is the first turnout,\n" +
			"shown as 1 in the Z21 app. It accepts decimal or 0x hex.",
		Example: "  z21-sim-ctl turnout set 0 2\n" +
			"  z21-sim-ctl turnout set 0x10 1",
		Args: cobra.ExactArgs(2),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 1 {
				return []string{"1", "2"}, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, err := parseAccessoryAddress(args[0])
			if err != nil {
				return err
			}
			var pos simclient.TurnoutPosition
			switch args[1] {
			case "1":
				pos = simclient.TurnoutOutput1
			case "2":
				pos = simclient.TurnoutOutput2
			default:
				return fmt.Errorf("invalid output %q: want 1 or 2", args[1])
			}

			return a.withClient(cmd, func(ctx context.Context, c *simclient.Client) error {
				turnout, recipients, err := c.SetTurnout(ctx, addr, pos)
				if err != nil {
					return err
				}
				r := turnoutSetResult{Address: turnout.Address, Recipients: recipients}
				switch turnout.Position {
				case simclient.TurnoutOutput1:
					r.Output = 1
				case simclient.TurnoutOutput2:
					r.Output = 2
				}
				return a.print(cmd, r, func(w io.Writer) {
					fmt.Fprintf(w, "turnout %d: output %d, sent to %s\n", r.Address, r.Output, plural(recipients, "client"))
				})
			})
		},
	}
}

func newAccessoryCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "accessory",
		Short: "Set extended accessory decoders as a throttle outside the Z21 LAN",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newAccessorySetCmd(a))
	return cmd
}

type accessorySetResult struct {
	Address    uint16 `json:"address"`
	Value      byte   `json:"value"`
	Recipients int    `json:"recipients"`
}

func newAccessorySetCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "set ADDR VALUE",
		Short: "Send a value to an extended accessory decoder",
		Long: "Set an extended accessory decoder as a throttle outside the Z21 LAN would. X-Bus\n" +
			"subscribers get LAN_X_EXT_ACCESSORY_INFO, also when the value stays the same.\n\n" +
			"ADDR is the address as on the wire (0 to 2047), VALUE the byte sent to the\n" +
			"decoder (0 to 255, for example a signal aspect). Both accept decimal or 0x hex.",
		Example: "  z21-sim-ctl accessory set 3 0x12\n" +
			"  z21-sim-ctl accessory set 0x10 0",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, err := parseAccessoryAddress(args[0])
			if err != nil {
				return err
			}
			value, err := strconv.ParseUint(args[1], 0, 8)
			if err != nil {
				return fmt.Errorf("invalid VALUE %q: want 0 to 255, decimal or 0x hex", args[1])
			}

			return a.withClient(cmd, func(ctx context.Context, c *simclient.Client) error {
				accessory, recipients, err := c.SetExtAccessory(ctx, addr, byte(value))
				if err != nil {
					return err
				}
				r := accessorySetResult{Address: accessory.Address, Value: accessory.Value, Recipients: recipients}
				return a.print(cmd, r, func(w io.Writer) {
					fmt.Fprintf(w, "accessory %d: value %d (0x%02x), sent to %s\n", r.Address, r.Value, r.Value, plural(recipients, "client"))
				})
			})
		},
	}
}

// parseAccessoryAddress parses a turnout or extended accessory address (FAdr)
// in decimal or 0x hex.
func parseAccessoryAddress(s string) (uint16, error) {
	n, err := strconv.ParseUint(s, 0, 16)
	if err != nil || n > maxAccessoryAddress {
		return 0, fmt.Errorf("invalid ADDR %q: want 0 to %d, decimal or 0x hex", s, maxAccessoryAddress)
	}
	return uint16(n), nil
}
