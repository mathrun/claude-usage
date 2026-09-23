// Package auth locates the Claude Code OAuth access token on the local machine.
package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// EnvVar is the environment variable that overrides all other token sources.
const EnvVar = "CLAUDE_CODE_OAUTH_TOKEN"

// keychainService is the macOS Keychain item name Claude Code stores its credentials under.
const keychainService = "Claude Code-credentials"

type credentials struct {
	ClaudeAiOauth struct {
		AccessToken string `json:"accessToken"`
		ExpiresAt   int64  `json:"expiresAt"` // unix millis
	} `json:"claudeAiOauth"`
}

// LoadToken finds the OAuth access token, in this order:
//  1. CLAUDE_CODE_OAUTH_TOKEN environment variable
//  2. macOS Keychain entry "Claude Code-credentials"
//  3. ~/.claude/.credentials.json
func LoadToken() (string, error) {
	if t := os.Getenv(EnvVar); t != "" {
		return t, nil
	}

	raw, err := readRaw()
	if err != nil {
		return "", err
	}
	return parse(raw, time.Now())
}

func readRaw() ([]byte, error) {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("security", "find-generic-password", "-s", keychainService, "-w").Output()
		if err == nil && len(bytes.TrimSpace(out)) > 0 {
			return out, nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(home, ".claude", ".credentials.json"))
	if err != nil {
		return nil, errors.New("no Claude Code credentials found; run `claude` and log in first")
	}
	return raw, nil
}

func parse(raw []byte, now time.Time) (string, error) {
	var creds credentials
	if err := json.Unmarshal(bytes.TrimSpace(raw), &creds); err != nil {
		return "", fmt.Errorf("parsing credentials: %w", err)
	}
	tok := creds.ClaudeAiOauth.AccessToken
	if tok == "" {
		return "", errors.New("credentials contain no access token; run `claude` and log in first")
	}
	if exp := creds.ClaudeAiOauth.ExpiresAt; exp > 0 && time.UnixMilli(exp).Before(now) {
		return "", errors.New("access token expired; start `claude` once to refresh it")
	}
	return tok, nil
}
