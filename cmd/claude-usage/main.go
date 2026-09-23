// Command claude-usage prints progress bars for your Claude usage limits
// (current session, current week across all models, and per-model limits).
//
// It reads the Claude Code OAuth token from the macOS Keychain
// (or ~/.claude/.credentials.json), calls the same usage endpoint that
// Claude Code's /usage command uses, and renders the result. By default it
// takes over the terminal (alternate screen), keeps running and refreshes
// the bars in place until Ctrl-C is pressed. Bars stretch to the full
// terminal width; the previous terminal content is restored on exit.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"claude-usage/internal/auth"
	"claude-usage/internal/render"
	"claude-usage/internal/usage"
)

const (
	ansiHideCursor = "\x1b[?25l"
	ansiShowCursor = "\x1b[?25h"
	// Alternate screen buffer: hides the normal screen (and its scrollback)
	// while we run, and restores it on exit, like less or htop.
	ansiAltScreenOn  = "\x1b[?1049h"
	ansiAltScreenOff = "\x1b[?1049l"
	ansiHome         = "\x1b[H"
	ansiClearDown    = "\x1b[J"
	ansiDim          = "\x1b[2m"
	ansiRed          = "\x1b[31m"
	ansiReset        = "\x1b[0m"
)

const (
	fallbackCols    = 80
	defaultInterval = 300 * time.Second
	minInterval     = 10 * time.Second
	maxBackoff      = 5 * time.Minute
)

type config struct {
	width    int // 0 = fit terminal
	color    bool
	asJSON   bool
	once     bool
	interval time.Duration
}

func main() {
	var cfg config
	flag.IntVar(&cfg.width, "w", 0, "bar width in characters (0 = fit terminal width)")
	noColor := flag.Bool("no-color", false, "disable ANSI colors")
	flag.BoolVar(&cfg.asJSON, "json", false, "print raw limits as JSON and exit")
	flag.BoolVar(&cfg.once, "once", false, "print once and exit instead of refreshing")
	cfg.interval = defaultInterval
	flag.Func("f", "refresh frequency in seconds (default 300; a duration like 1m30s also works)", func(v string) error {
		return parseInterval(v, &cfg.interval)
	})
	flag.Func("interval", "alias for -f", func(v string) error {
		return parseInterval(v, &cfg.interval)
	})
	flag.Parse()

	cfg.color = !*noColor && os.Getenv("NO_COLOR") == ""
	if cfg.interval < minInterval {
		cfg.interval = minInterval
	}
	// Live redraw only makes sense on a terminal.
	if cfg.asJSON || !term.IsTerminal(int(os.Stdout.Fd())) {
		cfg.once = true
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "claude-usage:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config) error {
	token, err := auth.LoadToken()
	if err != nil {
		return err
	}
	client := usage.NewClient()

	if cfg.once {
		resp, err := client.Fetch(ctx, token)
		if err != nil {
			return err
		}
		if cfg.asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(resp.Limits)
		}
		if len(resp.Limits) == 0 {
			fmt.Println("no usage limits reported for this account")
			return nil
		}
		render.Bars(os.Stdout, resp.Limits, render.Options{Width: barWidth(cfg, resp.Limits), Color: cfg.color})
		return nil
	}

	return watch(ctx, cfg, client, token)
}

// watch switches to the alternate screen, then fetches usage on an interval
// and redraws the bars until ctx is cancelled (Ctrl-C). It also redraws on
// resize. On exit the previous terminal content is restored.
//
// When the endpoint rate-limits us, the next fetch is delayed with
// exponential backoff (honoring Retry-After) instead of hammering it.
func watch(ctx context.Context, cfg config, client *usage.Client, token string) error {
	fmt.Print(ansiAltScreenOn + ansiHideCursor + ansiHome + ansiClearDown)
	defer fmt.Print(ansiShowCursor + ansiAltScreenOff)

	var (
		limits   []usage.Limit
		lastOK   time.Time
		lastErr  error
		nextTry  time.Time
		failures int
	)

	draw := func() {
		var buf bytes.Buffer
		buf.WriteString(ansiHome + ansiClearDown)
		if len(limits) > 0 {
			render.Bars(&buf, limits, render.Options{
				Width: barWidth(cfg, limits),
				Color: cfg.color,
				Now:   time.Now(),
			})
		}
		buf.WriteString(statusLine(cfg, lastOK, lastErr, nextTry) + "\n")
		os.Stdout.Write(buf.Bytes())
	}

	// fetch updates state and returns how long to wait before the next fetch.
	fetch := func() time.Duration {
		fctx, cancel := context.WithTimeout(ctx, cfg.interval)
		defer cancel()
		resp, err := client.Fetch(fctx, token)
		if err != nil {
			if ctx.Err() != nil {
				return cfg.interval
			}
			lastErr = err
			failures++
			delay := backoff(cfg.interval, failures)
			var rl *usage.RateLimitError
			if errors.As(err, &rl) && rl.RetryAfter > delay {
				delay = rl.RetryAfter
			}
			nextTry = time.Now().Add(delay)
			return delay
		}
		limits, lastOK, lastErr = resp.Limits, time.Now(), nil
		failures = 0
		nextTry = time.Now().Add(cfg.interval)
		return cfg.interval
	}

	delay := fetch()
	draw()
	if lastErr != nil && len(limits) == 0 && errors.Is(lastErr, usage.ErrUnauthorized) {
		return lastErr
	}

	resize := make(chan os.Signal, 1)
	signal.Notify(resize, syscall.SIGWINCH)
	defer signal.Stop(resize)

	timer := time.NewTimer(delay)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-resize:
			draw()
		case <-timer.C:
			delay = fetch()
			draw()
			timer.Reset(delay)
		}
	}
}

// parseInterval accepts a plain number of seconds ("90") or a Go duration
// ("1m30s") and stores the result in dst.
func parseInterval(v string, dst *time.Duration) error {
	if secs, err := strconv.Atoi(v); err == nil {
		if secs <= 0 {
			return fmt.Errorf("must be a positive number of seconds")
		}
		*dst = time.Duration(secs) * time.Second
		return nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("expected seconds (e.g. 60) or a duration (e.g. 1m30s)")
	}
	if d <= 0 {
		return fmt.Errorf("must be positive")
	}
	*dst = d
	return nil
}

// backoff returns interval * 2^(failures-1), capped at maxBackoff.
func backoff(interval time.Duration, failures int) time.Duration {
	d := interval
	for i := 1; i < failures && d < maxBackoff; i++ {
		d *= 2
	}
	if d > maxBackoff {
		d = maxBackoff
	}
	return d
}

// barWidth returns the configured bar width, or one that fills the terminal.
func barWidth(cfg config, limits []usage.Limit) int {
	if cfg.width > 0 {
		return cfg.width
	}
	cols, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || cols <= 0 {
		cols = fallbackCols
	}
	return render.FitWidth(cols, limits)
}

func statusLine(cfg config, lastOK time.Time, lastErr error, nextTry time.Time) string {
	var sb strings.Builder
	if cfg.color {
		sb.WriteString(ansiDim)
	}
	if lastOK.IsZero() {
		sb.WriteString("waiting for first update")
	} else {
		sb.WriteString("updated " + lastOK.Format("15:04:05"))
	}
	fmt.Fprintf(&sb, " · every %ds · Ctrl-C to quit", int(cfg.interval.Round(time.Second)/time.Second))
	if cfg.color {
		sb.WriteString(ansiReset)
	}
	if lastErr != nil {
		sb.WriteString("\n")
		if cfg.color {
			sb.WriteString(ansiRed)
		}
		var rl *usage.RateLimitError
		if errors.As(lastErr, &rl) {
			sb.WriteString("rate limited by the usage endpoint")
		} else {
			sb.WriteString("error: " + firstLine(lastErr.Error()))
		}
		if !nextTry.IsZero() {
			sb.WriteString(" · next try at " + nextTry.Format("15:04:05"))
		}
		if cfg.color {
			sb.WriteString(ansiReset)
		}
	}
	return sb.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
