# JSON command contract v1

Schema v1 covers operational leaf commands that deliberately render JSON and `raidnow --json version`. Every in-scope result is one compact JSON object on stdout followed by a newline.

Root or group help, `--help`, the explicit `help` command, shell completion, and Cobra's hidden completion protocols are human/Cobra surfaces. They are outside this contract and may emit normal Cobra text even when `--json` is present.

## Envelopes

An in-scope success has this shape:

```json
{"schemaVersion":1,"ok":true,"command":"version","status":"shown","data":{"version":"dev"},"warnings":[]}
```

An in-scope failure has this shape:

```json
{"schemaVersion":1,"ok":false,"command":"queue status","status":"failed","error":{"code":"NOT_FOUND","message":"Ingen kö hittades."}}
```

`schemaVersion` is always the integer `1`. Successes include command-specific `data` and a `warnings` array. Failures include a stable typed `error.code` and a sanitized `error.message`. Ordinary human error text is not also written to stderr for in-scope JSON failures. A broken output stream can prevent the object from being written.

## Exit codes

JSON does not change typed process exits:

| Exit | Meaning |
| ---: | --- |
| 0 | Success |
| 1 | Internal or output failure |
| 2 | Invalid command-line arguments |
| 3 | Invalid input |
| 4 | Authentication unsupported |
| 5 | Not found |
| 6 | Lifecycle conflict |
| 7 | Upstream rejection |
| 8 | Read/network failure |
| 9 | Write outcome unknown |
| 10 | Local persistence failure |
| 11 | Metadata unavailable |
| 12 | Partial success |
| 13 | Local security failure |

## Compatibility

Consumers must ignore unknown fields. V1 may add optional fields, but existing fields will not be removed or change type or meaning. Such a breaking change requires a new schema version.

Pin the CLI's Docker image version in deployments. `raidnow --json version` reports the CLI version in `data.version` for diagnostics and smoke tests; it does not replace image pinning.

## Secrets

Tokens, trainer codes, and recognized secrets are redacted from normal output and errors. Intentional secret-bearing successes are limited to `raidnow --json token get` and matched raid details from `raidnow --json raid get` or `raidnow --json raid status`. These responses mark `data.containsSecret: true` (for status, inside `data.raid`) and include a handling warning. Treat the entire stdout object as secret material. The CLI does not persist it. Lobby lists, boss lists, player output, and all human raid output omit the host trainer code.
