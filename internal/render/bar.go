// Package render draws usage limits as terminal progress bars.
package render

import (
	"fmt"
	"io"
	"strings"
	"time"

	"claude-usage/internal/usage"
)

// Options controls the output format.
type Options struct {
	Width int  // bar width in characters
	Color bool // emit ANSI colors
	Now   time.Time
}

// Bars writes one progress bar per limit.
func Bars(w io.Writer, limits []usage.Limit, opt Options) {
	if opt.Width < MinWidth {
		opt.Width = MinWidth
	}
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}

	labels := make([]string, len(limits))
	maxLabel := 0
	for i, l := range limits {
		labels[i] = Label(l)
		if n := len(labels[i]); n > maxLabel {
			maxLabel = n
		}
	}

	for i, l := range limits {
		pct := clamp(l.Percent)
		bar := Bar(pct, opt.Width)
		if opt.Color {
			bar = colorFor(pct) + bar + "\x1b[0m"
		}

		reset := ""
		if l.ResetsAt != nil {
			if t, err := time.Parse(time.RFC3339Nano, *l.ResetsAt); err == nil {
				reset = "  resets in " + HumanDuration(t.Sub(opt.Now))
			}
		}

		fmt.Fprintf(w, "%-*s  %s %3.0f%%%s\n", maxLabel, labels[i], bar, pct, reset)
	}
}

// MinWidth is the narrowest bar Bars will draw.
const MinWidth = 5

// resetSuffixWidth is the widest "  resets in 1d 20h" suffix we expect.
const resetSuffixWidth = len("  resets in 99d 23h")

// FitWidth returns the bar width that makes each line fill cols columns,
// given the labels of the limits to be drawn.
func FitWidth(cols int, limits []usage.Limit) int {
	maxLabel := 0
	for _, l := range limits {
		if n := len([]rune(Label(l))); n > maxLabel {
			maxLabel = n
		}
	}
	// label + 2 spaces + bar + space + "100%" + reset suffix + 1 margin
	w := cols - maxLabel - 2 - 1 - 4 - resetSuffixWidth - 1
	if w < MinWidth {
		return MinWidth
	}
	return w
}

// Bar returns a plain progress bar of the given width for pct in [0,100].
func Bar(pct float64, width int) string {
	filled := int(clamp(pct)/100*float64(width) + 0.5)
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// Label returns a human-readable name for a limit.
func Label(l usage.Limit) string {
	switch l.Kind {
	case "session":
		return "Current session"
	case "weekly_all":
		return "Current week (all models)"
	case "weekly_scoped":
		if l.Scope != nil && l.Scope.Model != nil && l.Scope.Model.DisplayName != "" {
			return "Current week (" + l.Scope.Model.DisplayName + ")"
		}
		return "Current week (scoped)"
	}
	return strings.ReplaceAll(l.Kind, "_", " ")
}

// HumanDuration formats a duration like "2h 05m" or "1d 3h".
func HumanDuration(d time.Duration) string {
	if d <= 0 {
		return "now"
	}
	d = d.Round(time.Minute)
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %02dm", hours, mins)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

func clamp(pct float64) float64 {
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

func colorFor(pct float64) string {
	switch {
	case pct >= 90:
		return "\x1b[31m" // red
	case pct >= 70:
		return "\x1b[33m" // yellow
	default:
		return "\x1b[32m" // green
	}
}
