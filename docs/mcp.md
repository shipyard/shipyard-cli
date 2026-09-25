# Shipyard MCP server

`shipyard mcp serve` runs a [Model Context Protocol](https://modelcontextprotocol.io)
server, so an AI assistant such as Claude Code, Claude Desktop, Cursor or Codex can
work with your Shipyard environments. Once it's connected, you can ask for things
in plain language:

- "Which environments are running for the `web` repo?"
- "Show me the last 200 lines of the `api` service's logs on my branch's environment."
- "Why did the latest build of this environment fail?"
- "Set `FEATURE_FLAGS=beta` on this environment and restart the `worker` service."
- "Point the `web` repo in this environment at the `fix-login` branch."
- "Snapshot the `postgres` volume before you run the migration."
- "I just pushed. Verify the change against its environment."

The server runs locally over stdio and uses the same token, org and config file
as the CLI.

- [Quick start](#quick-start)
- [Connecting a client](#connecting-a-client)
- [Configuration](#configuration)
- [Tools](#tools)
- [Resources](#resources)
- [The `verify` prompt](#the-verify-prompt)
- [Troubleshooting](#troubleshooting)

## Quick start

1. **Install the CLI, version 1.9.0 or later** (see [Installation](../README.md#installation)),
   and check it:

   ```bash
   shipyard --version
   ```

2. **Log in**, so the server can find your token:

   ```bash
   shipyard login
   ```

   If you belong to more than one org, set a default: `shipyard set org {org_name}`.

3. **Add the server to your assistant.** For Claude Code:

   ```bash
   claude mcp add shipyard -- shipyard mcp serve
   ```

   Other clients are covered in [Connecting a client](#connecting-a-client).

4. **Try it.** Start a new session and ask "list my Shipyard environments".

## Connecting a client

Every client starts the same command, `shipyard mcp serve`. If the CLI is already
logged in, the server reads the token and org from `~/.shipyard/config.yaml`, and
you don't need an `env` block. Pass `SHIPYARD_API_TOKEN` and `SHIPYARD_ORG` in the
client's environment when you want to set them per client, or when the client
can't read your config file.

`shipyard` has to be on the `PATH` the client sees. Desktop apps usually don't
inherit your shell's `PATH`, so use the absolute path (`which shipyard`) as the
command if the server fails to start.

### Claude Code

If the CLI is already logged in:

```bash
claude mcp add shipyard -- shipyard mcp serve
```

With an API token and org name:

```bash
claude mcp add shipyard --env SHIPYARD_API_TOKEN=your-token-here --env SHIPYARD_ORG=your-org-name -- shipyard mcp serve
```

Add `--scope user` to make the server available in every project, not just the
current one. Check the connection with `/mcp` inside Claude Code.

### Claude Desktop

Open Settings → Developer → Edit Config, which opens
`claude_desktop_config.json`, and add:

```json
{
  "mcpServers": {
    "shipyard": {
      "command": "/opt/homebrew/bin/shipyard",
      "args": ["mcp", "serve"]
    }
  }
}
```

Replace the command with the output of `which shipyard`. Add an `env` block, as
in the Cursor example below, if the CLI isn't logged in. Restart Claude Desktop
afterwards.

### Cursor

Create `~/.cursor/mcp.json` for every project, or `.cursor/mcp.json` inside one
project, and add:

```json
{
  "mcpServers": {
    "shipyard": {
      "command": "shipyard",
      "args": ["mcp", "serve"],
      "env": {
        "SHIPYARD_API_TOKEN": "your-token-here",
        "SHIPYARD_ORG": "your-org-name"
      }
    }
  }
}
```

Drop the `env` block if the CLI is already configured. Reload the window, then
check Settings → MCP, where Shipyard should list its tools and the `verify`
prompt.

### Codex CLI

Edit `~/.codex/config.toml` and add:

```toml
[mcp_servers.shipyard]
command = "shipyard"
args = ["mcp", "serve"]
env = { "SHIPYARD_API_TOKEN" = "your-token-here", "SHIPYARD_ORG" = "your-org-name" }
```

Drop the `env` line if the CLI is already configured.

### Other clients

Any MCP client that can launch a stdio server works. Configure it to run
`shipyard mcp serve`, with the environment variables below if needed.

## Configuration

The server reads the same settings as the CLI. Environment variables override the
config file.

| Setting | Config file key | Environment variable | Default |
|---|---|---|---|
| API token | `api_token` | `SHIPYARD_API_TOKEN` | none; set by `shipyard login` |
| Org | `org` | `SHIPYARD_ORG` | your default org |
| Allow `exec_service` to run commands | `mcp.allow_exec` | `SHIPYARD_MCP_ALLOW_EXEC` | `false` |
| Config file location | n/a | `--config {path}` flag | `~/.shipyard/config.yaml` |

A complete config file for the server looks like this:

```yaml
# ~/.shipyard/config.yaml
api_token: your-token-here
org: your-org-name
mcp:
  allow_exec: false
```

A few things differ from the CLI:

- **The `--org` flag isn't supported.** A server is long-running, so it uses one
  org for its whole life: set it with `SHIPYARD_ORG` or `org` in the config file.
  The assistant can change the default org with the `set_org` tool, which writes
  it to the config file the CLI uses too.
- **`SHIPYARD_MCP_ALLOW_EXEC` stays in the client that sets it.** It's never
  written back to the config file, so turning exec on in one client's `env`
  block doesn't turn it on for other clients. `false` in the environment
  overrides `true` in the file, and any value other than `true` or `false`
  leaves exec off.
- **Restart the client after changing settings.** The server reads its
  configuration once, at startup.

## Tools

### Environments

| Tool | What it does |
|---|---|
| `get_environments` | List environments, filtered by repo and branch, including deleted ones on request |
| `get_environment` | Get one environment's details, including its URL and per-repo commits |
| `stop_environment` | Stop a running environment |
| `restart_environment` | Restart a stopped environment |
| `rebuild_environment` | Rebuild with the latest commit |
| `cancel_environment` | Cancel the environment's latest build |
| `revive_environment` | Revive a deleted environment |

### Environment configuration

| Tool | What it does |
|---|---|
| `get_build_history` | List an environment's builds, optionally only successful ones |
| `get_env_vars` | List environment variables; hidden values are masked |
| `put_env_vars` | Create or update environment variables |
| `delete_env_var` | Delete an environment variable by name |
| `update_branches` | Change the branch of every repo in an environment |
| `deploy_detached` | Deploy a detached environment cloned from an application build |

### Services

| Tool | What it does |
|---|---|
| `get_services` | List an environment's services and exposed ports |
| `get_logs` | Get logs from a service |
| `restart_service` | Restart one service without rebuilding the environment |
| `exec_service` | Run a non-interactive command in a service container. Off by default; see below |

### Volumes

| Tool | What it does |
|---|---|
| `get_volumes` | List an environment's volumes |
| `reset_volume` | Reset a volume to its initial state |
| `get_snapshots` | List volume snapshots |
| `create_snapshot` | Create a volume snapshot |
| `load_snapshot` | Load a volume snapshot |

### Orgs

| Tool | What it does |
|---|---|
| `get_orgs` | List the orgs you belong to |
| `get_org` | Get the current default org |
| `set_org` | Set the default org |

### CLI-only operations

`port_forward` and `telepresence_connect` return the CLI command to run instead
of doing the work. Both need a connection that outlives a single request, so
the CLI is the right place for them:

```bash
shipyard port-forward --env {environment_uuid} --service {service} --ports {local}:{remote}
shipyard telepresence connect --env {environment_uuid}
```

### Running commands in a container (`exec_service`)

`exec_service` is disabled by default, because it's the only tool here that
runs arbitrary code inside a running environment. Turn it on per machine:

```yaml
# ~/.shipyard/config.yaml
mcp:
  allow_exec: true
```

Or in the MCP client's own environment, which is usually easier, since that's
where the rest of the server's settings live:

```
SHIPYARD_MCP_ALLOW_EXEC=true
```

For example, with Claude Code:

```bash
claude mcp add shipyard --env SHIPYARD_MCP_ALLOW_EXEC=true -- shipyard mcp serve
```

Restart the client afterwards. While it's off, `exec_service` explains how to
enable it and returns the equivalent `shipyard exec` command.

Once it's on, it returns stdout, stderr and the exit code. A command that exits
non-zero is a result, not an error. There is no terminal and no stdin, so
interactive programs (`bash`, `vim`, `psql` without `-c`) won't work; use
`shipyard exec` for those. After 60 seconds the tool closes the stream and
returns what the command has printed so far, with a null `exit_code`. Closing
the stream doesn't kill the process in the container: one that keeps writing
dies on the closed pipe, and one that never writes again runs until it exits.
Each stream is truncated past 64KB, with `truncated: true` in the response when
that happens.

## Resources

| URI | What it returns |
|---|---|
| `logs://{environment_id}/{service_name}?tail=100` | A service's recent logs |

Clients that support resource templates can read a service's logs by URI,
without a tool call.

## The `verify` prompt

`verify` checks a pushed change against its Shipyard environment. It waits for
the build of the pushed SHA, then checks the running environment before
reporting the change as working.

Clients that support prompts show it as a slash command, for example
`/mcp__shipyard__verify` in Claude Code. It takes one optional
argument, `acceptance`: the check that decides pass or fail. Everything else
comes from the working directory, the repository, or what you say in the
conversation: to verify another branch, say so ("verify branch `fix-login` of
`web`"), and the same goes for a read-only run (see
[Checking the change itself](#checking-the-change-itself)).

If the environment of the branch you have checked out is stopped, the agent
starts it once (a restart, or a rebuild if the restart is refused and no
build started) and says so in the report. It never starts any other
environment, including the base branch's or one for a branch you named: it
reports that it is stopped and leaves the decision to you.

The agent's commands fetch the environment's bypass token with
`shipyard get environment <id> --bypass-token` rather than typing it. That needs
a CLI with `--bypass-token` on the agent's `PATH`, logged in in the agent's
shell, not only in the MCP client's `env` block. If the fetch fails, the agent
types the token into its commands and says so in the report, with the fix:
upgrade or install the CLI, or run `shipyard login`.

The commands are POSIX shell, so they run as written on macOS, Linux, and Git
Bash or WSL on Windows. In PowerShell the agent uses the equivalent
`$env:SHIPYARD_TOKEN = ...; if ($? -and $LASTEXITCODE -eq 0 -and $env:SHIPYARD_TOKEN) { ... }`
form.

### The acceptance check

The prompt confirms the environment is serving your exact commit, then runs the
check that decides whether the change actually works. The check can be a
command, or a plain description of what should happen, which the agent turns
into a check (a `curl` request, an e2e test, a query in the service) and runs.
The report quotes your description next to the check it became. The prompt
looks for a check in two places:

1. The `acceptance` argument, for a single run. Always wrap it in quotes when it
   has spaces. Claude Code splits prompt arguments on every space, even inside
   quotes. An opening quote with no closing one tells the prompt the argument
   was cut, and the agent takes the full text you typed instead:

   ```
   /mcp__shipyard__verify "npm run test:e2e"
   /mcp__shipyard__verify "GET /api/widgets returns count as a number"
   /mcp__shipyard__verify "the signup button is blue"
   ```

   The agent treats it as a command when it reads as shell (a program or
   script followed by arguments, like `npm run test:e2e` or
   `CI=1 pytest -k login`), and as a description when it reads as a sentence,
   even one starting with a word like "make" or "test". When it could be
   either, it is a description, and the report says how it was read. A
   description is checked even in a read-only run, because you asked for it;
   one that can't be captured as a command ("the page feels faster") is
   reported as Observed, not Verified.

2. Whatever the repository documents, for every run. Put it somewhere the agent
   already reads, such as `CLAUDE.md` or `AGENTS.md`:

   ```markdown
   ## Verifying against Shipyard

   Acceptance check: `npm run test:e2e -- --base-url=$SHIPYARD_URL`
   ```

   The agent runs the check with `SHIPYARD_URL` set to the environment URL and
   `SHIPYARD_TOKEN` set to its bypass token. Send the token as the
   `shipyard_token` cookie and never print it. The check has to hit the
   environment; a unit-test command doesn't count.

With neither, the prompt still runs. It checks the change directly (below), or
reports that the environment is serving your commit and nothing was checked.
Nothing has to be configured to use the prompt.

### The test plan

Before it checks anything, the agent proposes a test plan and waits for you to
approve it. The plan covers the change and its blast radius: callers of changed
code, shared templates and components, access and permissions, data and
migrations, and other services in the environment. Each item says why it's at
risk (citing the diff), how it will be checked, whether it's new, changed or
unchanged behavior (an unchanged item is a guard: it needs no failure on the
base branch, and if it runs there it must pass), whether it writes data, and roughly how long it takes. Where the
client can start a subagent, a fresh one with no conversation history drafts
the plan, so it doesn't inherit the blind spots of the agent that wrote the
change; the plan says how it was drafted.

```
Test plan for 3f2a9c1 (drafted by a fresh subagent)

| # | Area              | Why at risk (diff lines)   | Check (tool + assertion)                    | New/changed | Writes data? | Rough time |
|---|-------------------|----------------------------|---------------------------------------------|-------------|--------------|------------|
| 1 | The change itself | routes/widgets.py:40-52    | curl GET /api/widgets, assert count is int  | new         | no           | 1 min      |
| 2 | Access            | route has no @login check  | curl without a token, assert 401            | new         | no           | 1 min      |
| 3 | Shared pieces     | partials/nav.html is shared | curl /settings, assert nav renders          | unchanged   | no           | 1 min      |

Reply: yes · drop 3 · add: <what> · change 2: <how> · no
```

Answer `yes`, adjust it (`drop 3`, `add: ...`, `change 2: ...`), or `no` to
stop without running anything. The environment keeps building while you read,
so approving costs little time. The report then lists every item: passed,
failed, skipped (with the reason) or dropped by you. Verified means every
approved item passed.

For unattended runs such as CI, add `Verification: auto-approve` to
`CLAUDE.md` or `AGENTS.md`, or say "auto-approve" when you run the prompt: the
agent then runs its own plan and includes it in the report. The agent reads
`Verification:` lines from the base branch, so a branch that adds the line
cannot approve its own plan; the line takes effect once it merges. A read-only run
skips the plan, since nothing gets added.

### Checking the change itself

A passing suite shows nothing regressed. It doesn't show the new behavior
works, because a new feature usually has no test yet. So the prompt also:

1. **Assesses coverage.** It reads the diff against the base branch and names
   the test that exercises each changed behavior: full, partial or none.
2. **Adds a check for anything uncovered**, using whatever tool reaches the
   change: an e2e test for UI work, an API test or `curl` requests for a
   backend change, a CLI call or a query through `exec_service` for a worker or
   migration. A check has to be reproducible: a test committed with the change,
   or every command quoted in the report with its expected and actual output.
   Something the agent only looked at is reported as Observed, never Verified.
3. **Proves checks for fixes are real.** For a bug fix or any change to
   behavior that already existed, a lazy check would pass before and after the
   change. So each new check for such a change must pass on this environment and
   fail on the base branch's environment, at its assertion. The base environment
   is used only if it's ready and serves exactly the commit your branch started
   from (merging the base branch into yours gets you there), only with
   checks that don't write, and is never restarted or edited. A brand-new feature
   skips this, and so does a new field or header on an existing route: base
   can't have it, so the check's quoted assertion on the new behavior is the
   proof instead.

The result is one of:

| Result | Meaning |
|---|---|
| Verified | Everything passed, every changed behavior is covered, and every added check for a fix failed on base |
| Passed, not covered | Everything passed, but some changed behavior has no proven check |
| Observed | The only evidence is something the agent looked at |
| Serving | The environment is up on your commit; nothing ran |
| FAILED | A check failed |

To keep verification read-only (no added checks, no edits to the running
container), add this line to `CLAUDE.md` or `AGENTS.md`:

```markdown
Verification: read-only
```

or ask for it in the conversation for one run ("verify this, but read-only").
Asking for checks in a read-only repository turns them back on for that run.

### Fixing without a rebuild per attempt

When `exec_service` is enabled (`mcp.allow_exec`) and the service runs a dev
server that reloads code, the agent can try a fix by writing changed files into
the running container, rerun its checks in seconds, and push once they pass. It
does this only after confirming the container serves the repository's files (by
comparing hashes) and runs a known reloading process. Results from an edited
container are never the verdict: after the push, the environment rebuilds and
the checks run again on the clean commit. If the agent stops without pushing, it
restores the files it changed.

## Troubleshooting

- **The client can't start the server, or says `shipyard` isn't found.** The
  client doesn't see your shell's `PATH`. Use the absolute path from
  `which shipyard` as the command.
- **The `verify` prompt or newer tools are missing.** An older CLI earlier on the
  `PATH`, often a Homebrew install, is answering instead. Check
  `shipyard --version` in the client's environment and upgrade with
  `brew upgrade shipyard`.
- **`exec_service` says it's disabled.** Set `SHIPYARD_MCP_ALLOW_EXEC=true` in the
  client's `env` block, or `mcp.allow_exec: true` in the config file, then
  restart the client.

- **The client reports a protocol or parse error on startup.** The server speaks
  JSON-RPC over stdout and logs to stderr; anything else writing to stdout
  breaks the session. Check with
  `echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}' | shipyard mcp serve`,
  whose first output line must be JSON.
- **Tools fail with a missing token.** Set `SHIPYARD_API_TOKEN` in the client's
  `env` block, or run `shipyard login` so the token lands in the config file.
  The client's environment is not your shell's.
- **A per-app firewall (LuLu, Little Snitch) is installed.** Approve the
  `shipyard` binary once, otherwise calls stall until the CLI's 20s timeout.
  Rules are keyed by path, so a Homebrew upgrade needs a fresh approval.
