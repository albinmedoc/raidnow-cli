# Raid NOW CLI

`raidnow` is a stateless, anonymous command-line client for Raid NOW.

This repository contains a working token bootstrap, the per-invocation trainer-input library, and stateless trainer publishing. It does not store accounts, profiles, tokens, trainer values, or workflow state.

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
./raidnow --token-file ./raidnow.token trainer publish \
  --trainer-name Ash123 --trainer-level 50 --trainer-country se \
  --trainer-code-file ./trainer-code
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

When reading `--token-file` on Unix, the CLI opens the descriptor with no-follow, nonblocking, and no-controlling-terminal flags, then verifies that the opened descriptor is a regular file owned by the effective user and inaccessible to group and others. These descriptor and ownership guarantees are Unix-specific. `trainer publish` consumes the token from `--token-file`; `token get` ignores `--token-file` because it fetches a new token. Human `token get` output is exactly the token plus a newline. JSON output deliberately contains the secret too, marks `status` as `secret`, sets `containsSecret: true`, and includes a handling warning. The CLI stores no token, cookies, CSRF state, account, profile, or workflow state.

The form transport is stateless: callers provide the token on each request, and write requests are never retried automatically. `--request-timeout <duration>` overrides an operation's default timeout; zero retains the registered default.

## Metadata cache

The Pokémon metadata cache uses the platform user-cache directory (or an absolute `XDG_CACHE_HOME` on supported systems). Every existing path component is checked without following symbolic links; Windows reparse points are rejected. On Unix, an existing cache directory must be owned by the effective user and must not be group- or world-writable. Newly created Unix cache directories are set to `0700` through a no-follow directory descriptor and cache files to `0600`. Windows does not provide portable Unix owner/mode semantics through Go, so directory access remains governed by the user's ACL while reparse-point and file-handle identity checks are still enforced.

Cache reads consume bytes only from a no-follow descriptor validated as a regular file. Writes use an owner-only temporary file, verify that its open descriptor still names the temporary directory entry, and atomically replace the cache; interrupted or adversarial replacement leaves the prior cache intact.

## Trainer input

`trainer publish` binds invocation-local `--trainer-name`, `--trainer-level`, `--trainer-country`, and `--trainer-code-file <path|->` flags. It validates the values only when the command runs, publishes them with the explicit anonymous token, and keeps the normalized trainer identity in process memory. Country values are normalized to lowercase and bounded to 16 UTF-8 bytes without imposing a fixed country catalog.

Supply the trainer code through an owner-only protected file, or use `--trainer-code-file -` to read it from stdin. The protected-file reader applies the same Unix descriptor, ownership, regular-file, and permission checks described above. Supply a trainer code this way, never a raw argv flag; command arguments are commonly visible to other local processes and shell history. The CLI never creates a trainer profile or writes trainer values to disk.

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
