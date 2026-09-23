// Package usage fetches rate-limit utilization from the Claude OAuth usage endpoint.
package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultURL is the endpoint Claude Code's /usage command reads from.
const DefaultURL = "https://api.anthropic.com/api/oauth/usage"

// Limit is one rate-limit bucket as reported by the API.
type Limit struct {
	Kind     string  `json:"kind"`  // session, weekly_all, weekly_scoped, ...
	Group    string  `json:"group"` // session, weekly
	Percent  float64 `json:"percent"`
	Severity string  `json:"severity"`
	ResetsAt *string `json:"resets_at"`
	Scope    *Scope  `json:"scope"`
}

// Scope narrows a limit to a model or surface.
type Scope struct {
	Model *struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
}

// Response is the subset of the usage payload this tool needs.
type Response struct {
	Limits []Limit `json:"limits"`
}

// RateLimitError is returned when the endpoint answers 429.
// RetryAfter is zero when the server gave no usable hint.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited by usage endpoint (retry after %s)", e.RetryAfter)
	}
	return "rate limited by usage endpoint"
}

// ErrUnauthorized is returned when the token is rejected.
var ErrUnauthorized = errors.New("unauthorized; start `claude` once to refresh the token")

// Client talks to the usage endpoint.
type Client struct {
	URL        string
	HTTPClient *http.Client
}

// NewClient returns a client with sane defaults.
func NewClient() *Client {
	return &Client{
		URL:        DefaultURL,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Fetch retrieves current usage using the given OAuth bearer token.
func (c *Client) Fetch(ctx context.Context, token string) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "claude-usage/1.0")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting usage: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrUnauthorized
	case http.StatusTooManyRequests:
		return nil, &RateLimitError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usage endpoint returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var out Response
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parsing usage response: %w", err)
	}
	return &out, nil
}

// parseRetryAfter handles both forms of the header: delay-seconds and HTTP-date.
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}
