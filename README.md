# Retros Against Humanity

A Cards Against Humanity-inspired party game for team retrospectives. Players fill in the blanks on prompt cards, vote on their favorites, and discuss the winners.

Built with Go's standard library, HTMX, and Server-Sent Events for real-time multiplayer.

## How it works

1. A host creates a session with custom prompt cards containing `{blank}` placeholders
2. Participants join via a 6-character code
3. The host draws a card — everyone submits their answer within the timer
4. Players vote on submissions, the winner is revealed, and the team discusses
5. Repeat until all cards are played

## Prerequisites

- [Go](https://go.dev/) 1.26+

## Build and run

```sh
go build -o rah-go .
./rah-go
```

The server starts on port `8080` by default. Override with the `PORT` environment variable:

```sh
PORT=3000 ./rah-go
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-log-level` | `info` | `debug`, `info`, `warn`, or `error` |
| `-log-format` | `text` | `text` or `json` |

## Development

Install [Air](https://github.com/air-verse/air) for live reload:

```sh
air
```

### Run tests

```sh
go test ./...
```

## Notes

- All state is in-memory — sessions are lost on restart
- Sessions expire after 24 hours and are cleaned up automatically
- No external dependencies or databases required
