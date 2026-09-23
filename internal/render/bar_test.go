package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"claude-usage/internal/usage"
)

func TestBar(t *testing.T) {
	tests := []struct {
		pct   float64
		width int
		want  string
	}{
		{0, 10, "░░░░░░░░░░"},
		{50, 10, "█████░░░░░"},
		{100, 10, "██████████"},
		{150, 4, "████"},
		{-5, 4, "░░░░"},
	}
	for _, tt := range tests {
		if got := Bar(tt.pct, tt.width); got != tt.want {
			t.Errorf("Bar(%v,%d) = %q, want %q", tt.pct, tt.width, got, tt.want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "now"},
		{-time.Minute, "now"},
		{5 * time.Minute, "5m"},
		{2*time.Hour + 5*time.Minute, "2h 05m"},
		{27 * time.Hour, "1d 3h"},
	}
	for _, tt := range tests {
		if got := HumanDuration(tt.d); got != tt.want {
			t.Errorf("HumanDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestLabel(t *testing.T) {
	scoped := usage.Limit{Kind: "weekly_scoped", Scope: &usage.Scope{}}
	scoped.Scope.Model = &struct {
		DisplayName string `json:"display_name"`
	}{DisplayName: "Fable"}

	tests := []struct {
		l    usage.Limit
		want string
	}{
		{usage.Limit{Kind: "session"}, "Current session"},
		{usage.Limit{Kind: "weekly_all"}, "Current week (all models)"},
		{scoped, "Current week (Fable)"},
		{usage.Limit{Kind: "weekly_scoped"}, "Current week (scoped)"},
		{usage.Limit{Kind: "some_other_kind"}, "some other kind"},
	}
	for _, tt := range tests {
		if got := Label(tt.l); got != tt.want {
			t.Errorf("Label(%q) = %q, want %q", tt.l.Kind, got, tt.want)
		}
	}
}

func TestFitWidth(t *testing.T) {
	limits := []usage.Limit{{Kind: "session"}, {Kind: "weekly_all"}}
	// label "Current week (all models)" = 25 cols; overhead = 2+1+4+19+1 = 27
	if got := FitWidth(100, limits); got != 100-25-27 {
		t.Errorf("FitWidth(100) = %d, want %d", got, 100-25-27)
	}
	if got := FitWidth(10, limits); got != MinWidth {
		t.Errorf("FitWidth(10) = %d, want MinWidth %d", got, MinWidth)
	}
}

func TestBars(t *testing.T) {
	now := time.Date(2026, 9, 21, 18, 0, 0, 0, time.UTC)
	reset := "2026-09-21T20:40:00Z"
	limits := []usage.Limit{
		{Kind: "session", Percent: 35, ResetsAt: &reset},
		{Kind: "weekly_all", Percent: 23},
	}

	var buf bytes.Buffer
	Bars(&buf, limits, Options{Width: 10, Color: false, Now: now})
	out := buf.String()

	want := []string{
		"Current session            ████░░░░░░  35%  resets in 2h 40m\n",
		"Current week (all models)  ██░░░░░░░░  23%\n",
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q\ngot:\n%s", w, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Error("expected no ANSI codes when Color is false")
	}
}
