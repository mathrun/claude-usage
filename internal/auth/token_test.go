package auth

import (
	"strconv"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour).UnixMilli()
	past := now.Add(-time.Hour).UnixMilli()

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{"valid", `{"claudeAiOauth":{"accessToken":"abc","expiresAt":` + itoa(future) + `}}`, "abc", false},
		{"no expiry", `{"claudeAiOauth":{"accessToken":"abc"}}`, "abc", false},
		{"expired", `{"claudeAiOauth":{"accessToken":"abc","expiresAt":` + itoa(past) + `}}`, "", true},
		{"missing token", `{"claudeAiOauth":{}}`, "", true},
		{"garbage", `not json`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parse([]byte(tt.raw), now)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("token = %q, want %q", got, tt.want)
			}
		})
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
