package z21simctl

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/trains-io/z21-sim/simclient"
)

func newCANCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "can",
		Short: "Simulate CAN devices (occupancy detectors, boosters)",
		Args:  cobra.NoArgs,
	}
	add := &cobra.Command{
		Use:   "add",
		Short: "Plug in a CAN device",
		Args:  cobra.NoArgs,
	}
	add.AddCommand(newCANAddDetectorCmd(a), newCANAddBoosterCmd(a))
	cmd.AddCommand(add)
	return cmd
}

type canAddResult struct {
	Kind       string `json:"kind"`
	NetID      uint16 `json:"net_id"`
	Recipients int    `json:"recipients"`
}

func newCANAddDetectorCmd(a *app) *cobra.Command {
	var d simclient.CANDetector
	cmd := &cobra.Command{
		Use:   "detector NETID",
		Short: "Plug in a CAN occupancy detector",
		Long: "Plug in a CAN occupancy detector with all ports free (with voltage). CAN detector\n" +
			"subscribers get one LAN_CAN_DETECTOR occupancy report per port. A device with the\n" +
			"same NetID is replaced.\n\n" +
			"NETID and the numeric flags accept decimal or 0x hex.",
		Example: "  z21-sim-ctl can add detector 0xdb04 --name \"Block 4\" --module-addr 60 --ports 8",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			netID, err := parseNetID(args[0])
			if err != nil {
				return err
			}
			d.NetID = netID
			return a.withClient(cmd, func(ctx context.Context, c *simclient.Client) error {
				recipients, err := c.AddCANDetector(ctx, d)
				if err != nil {
					return err
				}
				return a.printCANAdd(cmd, canAddResult{Kind: "detector", NetID: netID, Recipients: recipients})
			})
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&d.Name, "name", "", "device name (at most 15 bytes)")
	flags.Uint16Var(&d.ModuleAddr, "module-addr", 1, "module address")
	flags.Uint16Var(&d.PortCount, "ports", 1, "number of feedback ports (at most 256)")
	return cmd
}

func newCANAddBoosterCmd(a *app) *cobra.Command {
	var b simclient.CANBooster
	cmd := &cobra.Command{
		Use:   "booster NETID",
		Short: "Plug in a CAN booster",
		Long: "Plug in a CAN booster. CAN booster subscribers get its\n" +
			"LAN_CAN_BOOSTER_SYSTEMSTATE_CHGD. A device with the same NetID is replaced.\n\n" +
			"NETID and the numeric flags accept decimal or 0x hex.",
		Example: "  z21-sim-ctl can add booster 0xc101 --name \"Booster 1\" --output-port 2",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			netID, err := parseNetID(args[0])
			if err != nil {
				return err
			}
			b.NetID = netID
			return a.withClient(cmd, func(ctx context.Context, c *simclient.Client) error {
				recipients, err := c.AddCANBooster(ctx, b)
				if err != nil {
					return err
				}
				return a.printCANAdd(cmd, canAddResult{Kind: "booster", NetID: netID, Recipients: recipients})
			})
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&b.Name, "name", "", "device name (at most 15 bytes)")
	flags.Uint16Var(&b.OutputPort, "output-port", 1, "booster output port")
	return cmd
}

func (a *app) printCANAdd(cmd *cobra.Command, r canAddResult) error {
	return a.print(cmd, r, func(w io.Writer) {
		fmt.Fprintf(w, "%s %#04x added, sent to %s\n", r.Kind, r.NetID, plural(r.Recipients, "client"))
	})
}

// parseNetID parses a CAN NetID in decimal or 0x hex.
func parseNetID(s string) (uint16, error) {
	n, err := strconv.ParseUint(s, 0, 16)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("invalid NETID %q: want 1 to 65535 (0xffff), decimal or 0x hex", s)
	}
	return uint16(n), nil
}
