# Raid NOW CLI

`raidnow` is a stateless, anonymous command-line client for Raid NOW.

This repository currently contains the initial Cobra-based CLI skeleton. It does not store accounts, profiles, tokens, or workflow state.

## Requirements

- Go 1.27 or later

## Build

```sh
go build -o raidnow ./cmd/raidnow
```

## Usage

```sh
./raidnow --help
./raidnow version
./raidnow token get
```

Fetch a token into a user-owned file with owner-only permissions from the moment it is created:

```sh
(umask 077; raidnow token get > raidnow.token)
```

The subshell sets a restrictive creation mask before any secret bytes are written, avoiding a window where another user could read a newly created, overly permissive file. The shell and user own that file. The CLI stores no token, cookies, CSRF state, account, profile, or workflow state.

Development builds report `dev`. Release builds can inject a version at link time:

```sh
go build -ldflags "-X github.com/albinmedoc/raidnow-cli/internal/cli.Version=v0.1.0" -o raidnow ./cmd/raidnow
```

## Test

```sh
go test ./...
go vet ./...
```

## License

MIT
