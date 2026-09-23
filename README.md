# claude-usage

Tiny terminal tool that shows your Claude usage limits as progress bars:
the current 5-hour session, the current week across all models, and any
per-model weekly limit. It takes over the terminal like `htop` or `less`,
stretches the bars to the full terminal width, and refreshes them in place
until you press Ctrl-C. Your previous terminal content is restored on exit.

```
$ claude-usage
Current session            ██████████░░░░░░░░░░░░░░░░░░░░  35%  resets in 2h 41m
Current week (all models)  ███████░░░░░░░░░░░░░░░░░░░░░░░  23%  resets in 1d 19h
Current week (Fable)       █████████████░░░░░░░░░░░░░░░░░  45%  resets in 1d 19h
updated 21:06:32 · every 300s · Ctrl-C to quit
```

It reuses the login from Claude Code, so no extra setup is needed. The token
is looked up in this order:

1. `CLAUDE_CODE_OAUTH_TOKEN` environment variable
2. macOS Keychain entry `Claude Code-credentials`
3. `~/.claude/.credentials.json`

## Build

```
make            # build ./claude-usage
make install    # symlink ./claude-usage into /usr/local/bin (asks for sudo)
make uninstall  # remove the symlink
make run        # build and run
make lint       # gofmt + go vet
make clean
```

Or without make: `go build -o claude-usage ./cmd/claude-usage`

`make install` links the built binary rather than copying it, so after a
`make build` the installed command is already up to date. Override the
target with `TARGET_DIR=~/bin make install` for a location that needs no sudo.

## Usage

```
claude-usage                 # live view, refreshes every 300s, Ctrl-C to quit
claude-usage -f 120          # refresh every 120 seconds (minimum 10s)
claude-usage -f 2m           # durations work too; -interval is an alias for -f
claude-usage -once           # print once and exit
claude-usage -w 50           # fixed bar width instead of fitting the terminal
claude-usage -no-color       # plain output (NO_COLOR env is also honored)
claude-usage -json           # raw limits as JSON and exit
```

Bars resize with the terminal window. When stdout is not a terminal (piped or redirected) the tool prints once and
exits, as if `-once` were given. If a refresh fails, the last good bars stay
on screen and the error is shown below them. When the endpoint answers with
429 (rate limited), the tool backs off exponentially (up to 5 minutes,
honoring `Retry-After`) and shows when it will try again.

Bars turn yellow at 70% and red at 90%. If the token has expired, start
`claude` once so it refreshes the credentials.
