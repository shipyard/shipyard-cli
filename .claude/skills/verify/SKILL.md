---
name: verify
description: Build the Shipyard CLI and drive its commands and MCP server against a fake API to verify a change at runtime.
---

# Verifying shipyard-cli changes

Build: `make build` → `bin/shipyard` (this is also the binary the local MCP config runs).

## Isolate

Never touch the real `~/.shipyard/config.yaml`. Per run:

```bash
export HOME=$(mktemp -d) SHIPYARD_API_TOKEN=fake SHIPYARD_API_URL=http://127.0.0.1:<port>
```

The first command prints `Creating a default config.yaml in $HOME/.shipyard` on stderr; that is expected.

## Fake API

`tests/server` has fixed fixtures. When a probe needs a specific response, such as an empty field or a 404,
run a small `python3 http.server` on port 0 that answers `GET /environment/<id>` with
`{"data":{"id":..,"type":"application","attributes":{...}}}`. The CLI appends `?org=<org>`.

## MCP surface

Pipe JSON-RPC lines into `bin/shipyard mcp serve`: `initialize`, `notifications/initialized`, then
`prompts/list`, `prompts/get` (`verify`), `tools/list`. Keep stdin open for a moment
(`(cat; sleep 3) |`) so the responses get written before EOF. `initialize.result.instructions` holds
the server instructions.

## Gotchas

- `/opt/homebrew/bin/shipyard` comes before `bin/` on PATH and may be an older release. A
  prompt that tells agents to run plain `shipyard ...` gets that one. Check both.
- Running MCP servers keep the old binary until they are reconnected (`/mcp`).
- Prompt commands must also work in PowerShell. Test them with
  `docker run --rm --network none -v <dir>:/s mcr.microsoft.com/powershell:7.4-mariner-2.0-arm64 pwsh -NoProfile -File /s/t.ps1`.
  On Apple silicon the amd64 `:latest` image segfaults under qemu when a command is not found.
