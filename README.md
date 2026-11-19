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

Fetch a token into a uniquely created, user-owned file that cannot overwrite or follow an existing path:

```bash
token_file=$(mktemp ./raidnow-token.XXXXXX) || exit 1
[ -f "$token_file" ] && [ ! -L "$token_file" ] && [ -O "$token_file" ] || { rm -f -- "$token_file"; exit 1; }
chmod 600 "$token_file" || { rm -f -- "$token_file"; exit 1; }
[ "$(stat -c '%a' "$token_file")" = 600 ] || { rm -f -- "$token_file"; exit 1; }
raidnow token get > "$token_file" || { rm -f -- "$token_file"; exit 1; }
printf 'Token saved to %s\n' "$token_file"
```

`mktemp` creates a new unpredictable path exclusively, and the ownership and mode checks happen before any secret bytes are written. The command prints the resulting path; remove it when finished with `rm -f -- "$token_file"`. The CLI stores no token, cookies, CSRF state, account, profile, or workflow state.

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
