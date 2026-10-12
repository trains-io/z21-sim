package z21simctl

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/trains-io/z21-sim/simclient"
)

type clientResult struct {
	Addr           string    `json:"addr"`
	LocalAddr      string    `json:"local_addr"`
	BroadcastFlags string    `json:"broadcast_flags"`
	Locos          []uint16  `json:"locos"`
	LastSeen       time.Time `json:"last_seen"`
}

func newClientsCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "clients",
		Short: "List the Z21 LAN clients with an active session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withClient(cmd, func(ctx context.Context, c *simclient.Client) error {
				clients, err := c.Clients(ctx)
				if err != nil {
					return err
				}
				results := make([]clientResult, len(clients))
				for i, ci := range clients {
					results[i] = clientResult{
						Addr:           ci.Addr.String(),
						LocalAddr:      ci.LocalAddr.String(),
						BroadcastFlags: fmt.Sprintf("0x%08x", ci.BroadcastFlags),
						Locos:          ci.Locos,
						LastSeen:       ci.LastSeen,
					}
				}
				return a.print(cmd, results, func(w io.Writer) { writeClients(w, results, time.Now()) })
			})
		},
	}
}

func writeClients(w io.Writer, clients []clientResult, now time.Time) {
	if len(clients) == 0 {
		fmt.Fprintln(w, "no clients")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ADDR\tLOCAL ADDR\tBROADCAST FLAGS\tLOCOS\tLAST SEEN")
	for _, c := range clients {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s ago\n",
			c.Addr, c.LocalAddr, c.BroadcastFlags, formatLocos(c.Locos), now.Sub(c.LastSeen).Round(time.Second))
	}
	tw.Flush()
}

func formatLocos(locos []uint16) string {
	if len(locos) == 0 {
		return "-"
	}
	s := make([]string, len(locos))
	for i, l := range locos {
		s[i] = strconv.Itoa(int(l))
	}
	return strings.Join(s, ",")
}
