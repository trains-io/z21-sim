package z21simctl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/trains-io/z21-sim/simclient"
)

const maxLocoAddress = 9999

func newLocoCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "loco",
		Short: "Drive locos as a throttle outside the Z21 LAN",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newLocoSetCmd(a))
	return cmd
}

type locoSetResult struct {
	Address       uint16 `json:"address"`
	SpeedSteps    int    `json:"speed_steps"`
	Speed         uint8  `json:"speed"`
	EmergencyStop bool   `json:"emergency_stop"`
	Forward       bool   `json:"forward"`
	Functions     []int  `json:"functions"`
	Changed       bool   `json:"changed"`
	Recipients    int    `json:"recipients"`
}

func newLocoSetCmd(a *app) *cobra.Command {
	var (
		speed            uint8
		steps            int
		forward, reverse bool
		functions        []string
	)
	cmd := &cobra.Command{
		Use:   "set ADDR",
		Short: "Change the speed, direction or functions of a loco",
		Long: "Change a loco as a throttle outside the Z21 LAN would. Values that are not given\n" +
			"stay unchanged; if the loco changes, the clients subscribed to it get\n" +
			"LAN_X_LOCO_INFO.\n\n" +
			"--speed is a speed step: 0 stops, at most 14, 28 or 126 depending on the speed\n" +
			"step mode (--steps; a loco nobody has driven has 128). It ends an emergency stop.\n" +
			"--steps without --speed keeps the speed step, which must exist in the new mode.\n\n" +
			"ADDR (1 to 9999) accepts decimal or 0x hex.",
		Example: "  z21-sim-ctl loco set 3 --speed 40 --forward\n" +
			"  z21-sim-ctl loco set 3 -f 0=on -f 1=off\n" +
			"  z21-sim-ctl loco set 1234 --steps 28 --speed 10 --reverse -f 0=on,4=on",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, err := parseLocoAddress(args[0])
			if err != nil {
				return err
			}
			flags := cmd.Flags()
			var change simclient.LocoChange
			if flags.Changed("steps") {
				switch s := simclient.SpeedSteps(steps); s {
				case simclient.SpeedSteps14, simclient.SpeedSteps28, simclient.SpeedSteps128:
					change.SpeedSteps = s
				default:
					return fmt.Errorf("invalid --steps %d: want 14, 28 or 128", steps)
				}
			}
			if flags.Changed("speed") {
				change.Speed = &speed
			}
			if forward || reverse {
				change.Forward = &forward
			}
			if change.Functions, err = parseLocoFunctions(functions); err != nil {
				return err
			}
			if change.SpeedSteps == 0 && change.Speed == nil && change.Forward == nil && change.Functions == nil {
				return errors.New("nothing to change; give at least one of --speed, --forward, --reverse, --steps, --function")
			}

			return a.withClient(cmd, func(ctx context.Context, c *simclient.Client) error {
				loco, changed, recipients, err := c.SetLoco(ctx, addr, change)
				if err != nil {
					return err
				}
				r := locoSetResult{
					Address:       loco.Address,
					SpeedSteps:    int(loco.SpeedSteps),
					Speed:         loco.Speed,
					EmergencyStop: loco.EmergencyStop,
					Forward:       loco.Forward,
					Functions:     []int{},
					Changed:       changed,
					Recipients:    recipients,
				}
				for fn := range uint8(32) {
					if loco.Function(fn) {
						r.Functions = append(r.Functions, int(fn))
					}
				}
				return a.print(cmd, r, func(w io.Writer) {
					fmt.Fprintf(w, "loco %d: %s", r.Address, describeLoco(r))
					if changed {
						fmt.Fprintf(w, ", sent to %s\n", plural(recipients, "client"))
					} else {
						fmt.Fprintln(w, unchangedSuffix(false))
					}
				})
			})
		},
	}
	flags := cmd.Flags()
	flags.Uint8Var(&speed, "speed", 0, "speed step, 0 stops")
	flags.BoolVar(&forward, "forward", false, "drive forward")
	flags.BoolVar(&reverse, "reverse", false, "drive in reverse")
	flags.IntVar(&steps, "steps", 0, "speed step mode: 14, 28 or 128")
	flags.StringSliceVarP(&functions, "function", "f", nil, "switch function N (0 to 31) on or off: N=on|off; repeatable or comma-separated")
	cmd.MarkFlagsMutuallyExclusive("forward", "reverse")
	_ = cmd.RegisterFlagCompletionFunc("steps", cobra.FixedCompletions(
		[]string{"14", "28", "128"}, cobra.ShellCompDirectiveNoFileComp))
	return cmd
}

// describeLoco returns the direction, speed and functions of r in words.
func describeLoco(r locoSetResult) string {
	direction := "reverse"
	if r.Forward {
		direction = "forward"
	}
	maxStep := r.SpeedSteps
	if maxStep == int(simclient.SpeedSteps128) {
		maxStep = 126
	}
	speed := fmt.Sprintf("speed %d/%d", r.Speed, maxStep)
	if r.EmergencyStop {
		speed = fmt.Sprintf("emergency stop (%d speed steps)", r.SpeedSteps)
	}
	functions := "functions off"
	if len(r.Functions) > 0 {
		names := make([]string, len(r.Functions))
		for i, fn := range r.Functions {
			names[i] = fmt.Sprintf("F%d", fn)
		}
		functions = strings.Join(names, " ") + " on"
	}
	return direction + ", " + speed + ", " + functions
}

// parseLocoAddress parses a loco address in decimal or 0x hex.
func parseLocoAddress(s string) (uint16, error) {
	n, err := strconv.ParseUint(s, 0, 16)
	if err != nil || n == 0 || n > maxLocoAddress {
		return 0, fmt.Errorf("invalid ADDR %q: want 1 to %d, decimal or 0x hex", s, maxLocoAddress)
	}
	return uint16(n), nil
}

// parseLocoFunctions parses N=on|off values of --function; nil if there are
// none.
func parseLocoFunctions(values []string) (map[uint8]bool, error) {
	if len(values) == 0 {
		return nil, nil
	}
	functions := make(map[uint8]bool, len(values))
	for _, v := range values {
		num, state, ok := strings.Cut(v, "=")
		fn, err := strconv.ParseUint(num, 10, 8)
		if !ok || err != nil || fn > 31 || (state != "on" && state != "off") {
			return nil, fmt.Errorf("invalid --function %q: want N=on or N=off with N from 0 to 31", v)
		}
		functions[uint8(fn)] = state == "on"
	}
	return functions, nil
}
