package usage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"limits":[{"kind":"session","group":"session","percent":35,"resets_at":"2026-09-21T20:40:00Z"}]}`))
	}))
	defer srv.Close()

	c := NewClient()
	c.URL = srv.URL

	got, err := c.Fetch(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Limits) != 1 || got.Limits[0].Kind != "session" || got.Limits[0].Percent != 35 {
		t.Fatalf("unexpected response: %+v", got)
	}

	if _, err := c.Fetch(context.Background(), "wrong"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestFetchRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rate_limit_error"}}`))
	}))
	defer srv.Close()

	c := NewClient()
	c.URL = srv.URL

	_, err := c.Fetch(context.Background(), "tok")
	var rl *RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("expected *RateLimitError, got %v", err)
	}
	if rl.RetryAfter != 7*time.Second {
		t.Fatalf("RetryAfter = %s, want 7s", rl.RetryAfter)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"0", 0},
		{"30", 30 * time.Second},
		{"garbage", 0},
		{now.Add(90 * time.Second).Format(http.TimeFormat), 90 * time.Second},
		{now.Add(-time.Minute).Format(http.TimeFormat), 0},
	}
	for _, tt := range tests {
		if got := parseRetryAfter(tt.in, now); got != tt.want {
			t.Errorf("parseRetryAfter(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}
