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

Fetch a token with a single shell open into a new owner-only file:

```bash
token_file=./raidnow.token
if (umask 077; set -C; exec raidnow token get > "$token_file"); then
  printf 'Token saved to %s\n' "$token_file"
else
  printf 'Token was not saved; remove any partial file explicitly before retrying.\n' >&2
  exit 1
fi
```

Run this in a user-owned, non-world-writable directory. On supported Bash/POSIX shells, noclobber refuses an existing regular file and a symbolic link to a regular file, and the producer is not started when that destination check fails. If the producer fails after creation, an owner-only partial file can remain; inspect it if appropriate and explicitly remove it before retrying. Shell redirection does not provide a universal special files or `O_NOFOLLOW` guarantee, so choose the destination directory and path yourself rather than treating this snippet as a general secure-file API.

When reading `--token-file` on Unix, the CLI opens the descriptor with no-follow, nonblocking, and no-controlling-terminal flags, then verifies that the opened descriptor is a regular file owned by the effective user and inaccessible to group and others. These descriptor and ownership guarantees are Unix-specific. The CLI stores no token, cookies, CSRF state, account, profile, or workflow state.

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
