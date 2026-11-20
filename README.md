# Raid NOW CLI

`raidnow` is a stateless, anonymous command-line client for Raid NOW.

The usable commands fetch anonymous tokens, publish invocation-scoped trainer information, and perform single queue, lobby, and raid operations. The CLI does not store accounts, profiles, tokens, trainer values, or workflow state.

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
./raidnow --token-file ./raidnow.token queue join \
  --trainer-name Ash123 --trainer-level 50 --trainer-country se \
  --trainer-code-file ./trainer-code --rank 5 --pokemon 150 \
  --min-host-rating 3 --host-country 0 --weather-boost 0 --host-level 0
./raidnow --token-file ./raidnow.token queue status --rank 5 --pokemon 150
./raidnow --token-file ./raidnow.token queue wait --rank 5 --pokemon 150
./raidnow --token-file ./raidnow.token queue cancel --pokemon 150
./raidnow --token-file ./raidnow.token search \
  --trainer-name Ash123 --trainer-level 50 --trainer-country se \
  --trainer-code-file ./trainer-code --rank 5 --pokemon 150 \
  --min-host-rating 3 --host-country 0 --weather-boost 0 --host-level 0
./raidnow --token-file ./raidnow.token bosses list
./raidnow --token-file ./raidnow.token bosses list --rank 5 --json
./raidnow --token-file ./raidnow.token lobbies list --page 1 --rank 0
./raidnow --token-file ./raidnow.token raid join --raid-id RAID_ID \
  --trainer-name Ash123 --trainer-level 50 --trainer-country se
./raidnow --token-file ./raidnow.token raid status --raid-id RAID_ID
./raidnow --token-file ./raidnow.token raid leave --raid-id RAID_ID
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

When reading `--token-file`, the CLI requires a regular file, reads a bounded value and never modifies the file. Symbolic links, directories and other non-regular paths are rejected. `trainer publish` consumes the token from `--token-file`; `token get` ignores `--token-file` because it fetches a new token. Human `token get` output is exactly the token plus a newline. JSON output deliberately contains the secret too, marks `status` as `secret`, sets `containsSecret: true`, and includes a handling warning. The CLI stores no token, cookies, CSRF state, account, profile, or workflow state.

The form transport is stateless: callers provide the token on each request. Mutating commands are never retried automatically because a broken connection after dispatch can leave the write outcome unknown. Queue cancellation and raid leaving perform one read-back verification after an acknowledged write. `--request-timeout <duration>` overrides an operation's default timeout; zero retains the registered default.

`queue wait` only polls an existing Auto Join queue and never cancels it on timeout or interruption. `search` creates the queue once, waits in the foreground, and makes one best-effort cancel-and-check cleanup attempt after timeout or interruption. A successful match is the Auto Join result; inspect it with `raid status` when needed.

`queue join` and `search` accept the same Auto Join filters: `--min-host-rating` (`-1` through `5`, default `3`), `--host-country` (`0` or `1`, default `0`), `--weather-boost` (`0`, `1`, or `2`, default `0`), and `--host-level` (`0`, `40`, `50`, `60`, or `70`, default `0`). A minimum host rating of `-1` disables the host-rating filter.

## Pokémon names and IDs

`bosses list` lists the current Raid NOW boss catalog. Its JSON entries include the exact `pokemonId`, English `pokemonName`, `rank`, `waitingCount`, and the catalog's `imageUrl` (which may be an empty string):

```json
{"rank":"5","pokemonId":"150","pokemonName":"Armored Mewtwo","imageUrl":"https://example.invalid/pokemon/150.png","waitingCount":4}
```

`lobbies list` and `raid status` also include a `pokemonName` alongside the exact `pokemonId`. Names are the exact English values from Raid NOW's server-rendered `aryPokemon.en` catalog; form descriptors are not translated or rewritten.

The homepage metadata is fetched on demand only when a non-empty result needs names. It is kept in memory for that invocation and is never cached or written to disk. If metadata is unavailable, an ID is missing from the catalog, or its English name is blank, the command fails with `METADATA_UNAVAILABLE` instead of presenting the ID as a name. A blank boss image URL remains valid and is serialized as `"imageUrl":""`.

## Trainer input

`trainer publish` binds invocation-local `--trainer-name`, `--trainer-level`, `--trainer-country`, and `--trainer-code-file <path|->` flags. It validates the values only when the command runs, publishes them with the explicit anonymous token, and keeps the normalized trainer identity in process memory. Country values are normalized to lowercase and bounded to 16 UTF-8 bytes without imposing a fixed country catalog.

Supply the trainer code through a regular file, or use `--trainer-code-file -` to read it from stdin. Supply a trainer code this way, never a raw argv flag; command arguments are commonly visible to other local processes and shell history. You remain responsible for the file's local permissions. The CLI never creates a trainer profile or writes trainer values to disk.

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
