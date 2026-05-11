package main

import (
	"fmt"
	"time"

	"github.com/d56de/shrike/internal/stats"
	"github.com/spf13/cobra"
)

var (
	statsWeeks  int
	statsMetric string
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Activity heatmap of shrike usage over time",
	Long: `Render a GitHub-style activity heatmap from the local history file,
showing how active you've been running shrike. Defaults to the last 13 weeks
(roughly one quarter) of scan counts; pass --weeks to change the window.

Examples:
  shrike stats                       # last 13 weeks of scan activity
  shrike stats --weeks 26            # half-year view
  shrike stats --metric findings     # colour by finding count instead`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if statsWeeks < 1 || statsWeeks > 104 {
			return fmt.Errorf("--weeks must be between 1 and 104 (got %d)", statsWeeks)
		}
		switch statsMetric {
		case "scans", "findings", "high":
		default:
			return fmt.Errorf("--metric must be one of: scans, findings, high (got %q)", statsMetric)
		}

		now := time.Now()
		to := now
		// Anchor the right edge on the user's current day-of-week so the
		// grid always ends on a "today" cell instead of leaving a half-empty
		// trailing column.
		from := to.AddDate(0, 0, -7*statsWeeks+1)

		days, summary, err := stats.Aggregate(from, to)
		if err != nil {
			return fmt.Errorf("read history: %w", err)
		}

		title := fmt.Sprintf("Shrike activity — last %d weeks (%s – %s)",
			statsWeeks,
			summary.From.Format("Jan 02"),
			summary.To.Format("Jan 02 2006"))
		fmt.Println(title)
		fmt.Println()
		fmt.Print(stats.Render(days, summary, stats.RenderOptions{Metric: statsMetric}))
		return nil
	},
}

func init() {
	statsCmd.Flags().IntVar(&statsWeeks, "weeks", 13, "number of weeks to display (1-104)")
	statsCmd.Flags().StringVar(&statsMetric, "metric", "scans",
		"which metric drives the cell colour: scans, findings, high")
}
