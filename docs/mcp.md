# Shipyard MCP server

`shipyard mcp serve` runs a [Model Context Protocol](https://modelcontextprotocol.io)
server, so an AI assistant such as Claude Code, Claude Desktop, Cursor, Codex or Kiro
can work with your Shipyard environments. Once it's connected, you can ask for things
in plain language:

- "Which environments are running for the `web` repo?"
- "Show me the last 200 lines of the `api` service's logs on my branch's environment."
- "Why did the latest build of this environment fail?"
- "Set `FEATURE_FLAGS=beta` on this environment and restart the `worker` service."
- "Point the `web` repo in this environment at the `fix-login` branch."
- "Snapshot the `postgres` volume before you run the migration."
- "I just pushed. Verify the change against its environment."

The server runs locally over stdio and uses the same token, org and config file
as the CLI. `shipyard mcp host` runs the [hosted variant](#the-hosted-server)
for clients that connect over HTTP with OAuth.

- [Quick start](#quick-start)
- [Connecting a client](#connecting-a-client)
- [Configuration](#configuration)
- [Permissions and least privilege](#permissions-and-least-privilege)
- [Tools](#tools)
- [Resources](#resources)
- [The `verify` prompt](#the-verify-prompt)
- [The hosted server](#the-hosted-server)
- [Updating, uninstalling and rolling back](#updating-uninstalling-and-rolling-back)
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

**Keep your token out of project files.** `.cursor/mcp.json`, `.kiro/settings/mcp.json`
and Claude Code's `.mcp.json` live in the repository and tend to get committed.
Run `shipyard login` instead, or put the token in a per-user file such as
`~/.cursor/mcp.json`. The examples below show an `env` block only for the cases
where you need one.

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

Drop the `env` line if the CLI is already configured. You can also add it from the
command line: `codex mcp add shipyard -- shipyard mcp serve`, and check it with
`codex mcp list`.

### Kiro

The Kiro IDE and the Kiro CLI read the same files: `~/.kiro/settings/mcp.json`
for every workspace, or `.kiro/settings/mcp.json` inside one workspace, which wins
over the user file. Add:

```json
{
  "mcpServers": {
    "shipyard": {
      "command": "shipyard",
      "args": ["mcp", "serve"]
    }
  }
}
```

Kiro reloads the file when you save it. If the CLI isn't logged in, add an `env`
block as in the Cursor example, in the user file rather than the workspace one.
Before using Kiro unattended, read [Kiro](#kiro-1) under permissions: Kiro's CLI
doesn't ask before running a tool.

### Other clients

Any MCP client that can launch a stdio server works. Configure it to run
`shipyard mcp serve`, with the environment variables below if needed.

### Tested clients

These were run against a Shipyard org on 2026-10-09 and 2026-10-15, each with
a read-only session (list orgs, list environments) and a check that destructive
tools stay blocked:

| Client | Version tested |
|---|---|
| Claude Code | 2.1.284 |
| Codex CLI | 0.162.0 |
| Cursor | 3.24.9, and its CLI `cursor-agent` 2026.10.01 |
| Kiro | 1.2.56, and its CLI `kiro-cli` 2.28.0 |

Newer client versions should work; the server speaks MCP protocol versions
`2025-06-18`, `2025-03-26` and `2024-11-05`.

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

## Permissions and least privilege

### Which token to use

The server can do whatever its token can. `shipyard login` stores your personal
token, which reaches every org you belong to. For a shared machine, CI, or an
assistant you want confined to one org, use an **org API key** instead: it only
reaches that org. Set it as `SHIPYARD_API_TOKEN` in the client's `env` block, in
a per-user config file. An org has one key, and generating a new one replaces it,
so check with your org before rotating it.

`exec_service` stays off unless you turn it on (see
[Running commands in a container](#running-commands-in-a-container-exec_service)).

### What each tool may change

From 1.10.0, every tool tells the client whether it only reads, may destroy or
overwrite state, or neither. Clients use this to decide what to ask you about.

| Kind | Tools |
|---|---|
| Read-only | `get_environments`, `get_environment`, `get_build_history`, `get_failure_details`, `get_build_logs`, `get_env_vars`, `get_logs`, `get_services`, `get_volumes`, `get_snapshots`, `get_orgs`, `get_org`, `plan_application`, `port_forward` (returns a command, runs nothing) |
| Destructive: overwrites, stops or removes something | `stop_environment`, `cancel_environment`, `rebuild_environment`, `update_branches`, `restart_service`, `put_env_vars`, `delete_env_var`, `reset_volume`, `load_snapshot`, `exec_service`, `set_org` (changes the org every later call targets), `telepresence_connect` (rewrites `~/.shipyard/kubeconfig` and your routes), `update_application` (overwrites an app's config and can unlink repos), `create_application` (replaces an earlier create whose first build never started) |
| Adds state only | `restart_environment`, `revive_environment`, `deploy_detached`, `create_snapshot` |

### Letting an assistant read without asking

In an interactive session each client asks before a tool call, and you can
approve as you go. For an assistant that runs on its own (a script, a CI job, a
headless session), allow the read-only tools and nothing else. Each recipe below
was checked: the read tools run, and a destructive call is refused.

The `get_*` rules don't match `port_forward`, which is read-only too; add it by
name if you want it. An assistant that also starts environments to test against,
such as one running the [`verify` prompt](#the-verify-prompt), needs
`restart_environment` and `revive_environment` as well: environments stop after
a while without traffic, and these two only add state.

#### Claude Code

Add the read tools to `permissions.allow` in `.claude/settings.json` (or pass the
same rule to `claude -p --allowedTools`):

```json
{
  "permissions": {
    "allow": ["mcp__shipyard__get_*"]
  }
}
```

Every other Shipyard tool still needs your approval, and a non-interactive
session refuses it.

#### Codex

In `~/.codex/config.toml`, let Codex decide from the tool hints (CLI 1.10.0 or
later):

```toml
[mcp_servers.shipyard]
command = "shipyard"
args = ["mcp", "serve"]
default_tools_approval_mode = "writes"
```

Read tools then run without asking, and destructive tools aren't offered to a
session that can't ask you. With an older CLI, which sends no hints, approve each
read tool instead:

```toml
[mcp_servers.shipyard.tools.get_orgs]
approval_mode = "approve"

[mcp_servers.shipyard.tools.get_environments]
approval_mode = "approve"
```

#### Cursor

The Cursor agent CLI (`cursor-agent -p`) doesn't use the tool hints. Allow the
read tools in `.cursor/cli.json`, or `~/.cursor/cli-config.json` for every
project:

```json
{
  "permissions": {
    "allow": ["Mcp(shipyard:get_*)"]
  }
}
```

Cursor also asks you to approve the server itself the first time, and again
whenever its `command` changes, for example after an upgrade moves the binary.
`cursor-agent mcp enable shipyard` approves it from the command line.

#### Kiro

The Kiro CLI's non-interactive mode (`kiro-cli chat --no-interactive`) runs MCP
tools **without asking**, whatever `--trust-tools` says, and it ignores the tool
hints. Remove the destructive tools from the server with `disabledTools`:

```json
{
  "mcpServers": {
    "shipyard": {
      "command": "shipyard",
      "args": ["mcp", "serve"],
      "disabledTools": [
        "stop_environment", "cancel_environment", "rebuild_environment",
        "update_branches", "restart_service", "put_env_vars", "delete_env_var",
        "reset_volume", "load_snapshot", "exec_service", "set_org",
        "telepresence_connect", "update_application", "create_application"
      ]
    }
  }
}
```

A disabled tool is not offered to the model at all. This leaves the read tools
and the four that only add state (`restart_environment`, `revive_environment`,
`deploy_detached`, `create_snapshot`) on, so the assistant can start a stopped
environment to test against. Add those four too for a strictly read-only session.

`disabledTools` is a block-list: a tool added in a later release is enabled until
you add it, so check the [table above](#what-each-tool-may-change) after
upgrading. Put it on the `shipyard` entry Kiro actually uses: a workspace
`.kiro/settings/mcp.json` that defines `shipyard` wins over the user file.

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
| `get_build_history` | List an environment's builds, with a failure summary for failed ones (up to 100 per page) |
| `get_env_vars` | List environment variables; hidden values are masked |
| `put_env_vars` | Create or update environment variables |
| `delete_env_var` | Delete an environment variable by name |
| `update_branches` | Change the branch of every repo in an environment |
| `deploy_detached` | Deploy a detached environment cloned from an application build |

### Applications

| Tool | What it does |
|---|---|
| `plan_application` | Preview creating an app for a pushed GitHub branch: its compose file, services, suggested routes, the env vars the compose file needs set (`optional` when it has a default), any existing app, `app_name` (a name free in the org whose URL is free too) and `visitors_required` (true for an org API token). Changes nothing |
| `create_application` | Create an app from a confirmed plan: services (at least one), routes and env vars. Requires the plan's `head_sha`, so Shipyard builds the commit you confirmed. Each env item needs the services the plan lists for it, so a value reaches only those services, or `[]` when the plan lists none (a variable the compose file interpolates, set for the whole app); values are sent to Shipyard but never returned. Pass the plan's `app_name`; without one Shipyard picks a free name the same way. A service behind a compose profile needs that profile in `selected_profiles`. With an org API token, pass `visitors` (GitHub logins) |
| `update_application` | Reconfigure an app: a repo's branch, services, routes (`remove_routes` removes one) or `selected_profiles`, added or removed repos. Runtime env vars are set with `put_env_vars`, build args in the dashboard |

When Shipyard needs something done first, such as installing the GitHub app,
pushing the branch, adding a compose file or freeing an environment under your
plan's limit, these return `status: needs_action` with a `next_step` (`kind`,
`message`, and a `url` when there is one) instead of an error. The agent shows
you the message and waits for you to act.

A branch that already has an app returns `status: already_exists` with that app
and `requested_vs_existing`, the services, routes and env names you asked for
that it doesn't have yet; the agent can apply services and routes with
`update_application`, runtime env vars with `put_env_vars`, and build args in
the Shipyard dashboard. None of the request's env values are applied.
If another request is still creating the same app, `create_application` returns
`status: creating` and the agent tries again shortly.

### Failed builds

| Tool | What it does |
|---|---|
| `get_failure_details` | Why a build failed, in one call: phase and reason, failing services, excerpts from their build or crash logs, which services are enabled, and for a config or branch error Shipyard's description of it |
| `get_build_logs` | A build's stored build, run or crash logs, paged back from the end. Works after the environment stopped |

Both work on the latest build by default; pass a `build_id` from
`get_build_history` for an older one. Some failures are reported only as worth one
rebuild, without details; the agent checks that no build is already running first.
Hidden env var and secret values are masked in returned logs. Each log line is
prefixed with `| `, so the agent can tell the app's output from the tool's own
instructions. A block of lines repeated back to back, like the same stack trace
from every restart of a crash loop, is shown once with a count.

### Services

| Tool | What it does |
|---|---|
| `get_services` | List an environment's enabled services and exposed ports; a service that isn't listed is disabled |
| `get_logs` | Get live logs from a service in a running environment; `previous: true` returns the crashed container's last run (an init container or sidecar if that is what crashed), with its restart count, exit code and termination reason |
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

## The hosted server

`shipyard mcp host` serves a subset of the tools over stateless Streamable HTTP
for clients that sign in with OAuth instead of using a local token. It's what
Shipyard deploys as its hosted MCP endpoint; you don't need it to use the
local server.

- **Auth.** Every request needs `Authorization: Bearer` with a token Shipyard
  issued for this server. Without one it answers 401 with a `WWW-Authenticate`
  header pointing at its protected resource metadata
  (`/.well-known/oauth-protected-resource/<path>`), which names Shipyard as the
  authorization server, so clients start sign-in themselves.
- **Token exchange.** The server never sends the client's token to the API. It
  exchanges it at Shipyard's token endpoint (RFC 8693) for a short-lived API
  token, reused until shortly before it expires. If Shipyard can't be reached,
  tool calls fail with a retry hint rather than a 401, which would send the
  client back through sign-in; a cached token keeps working until it expires.
  A token over 4KB is refused with a 401 without asking Shipyard.
- **Tools.** Only the tools in `pkg/mcp/tools/scopes.go` are hosted, each behind
  the scope it needs: `plan_application`, `create_application`
  (`apps:create`), `update_application` (`apps:update`), and
  `get_environments`, `get_environment`, `get_build_history`,
  `get_failure_details`, `get_build_logs` (`envs:read`). Tools that reach your
  machine or a cluster directly, such as `exec_service`, `port_forward` and
  `telepresence_connect`, are never hosted. `bypass_token` is returned only
  with `envs:preview_access`.
- **Org.** A hosted token is bound to the org the user picked when connecting.

It's configured from the environment only, so the secret never lands in a
config file:

| Variable | Meaning |
|---|---|
| `SHIPYARD_MCP_RESOURCE_URL` | This server's MCP URL, e.g. `https://mcp.example.com/mcp`. Required; Shipyard's `OAUTH_MCP_RESOURCE` must match. |
| `SHIPYARD_MCP_SITE_URL` | Shipyard's URL. Default `https://shipyard.build`. |
| `SHIPYARD_MCP_CLIENT_ID` | This server's OAuth client id. Default `shipyard-mcp-host`. |
| `SHIPYARD_MCP_CLIENT_SECRET` | This server's OAuth client secret. Required; Shipyard's `OAUTH_MCP_HOST_CLIENT_SECRET` must match. |

```bash
shipyard mcp host --listen :8080
```

At startup it checks its client id, secret and URLs with Shipyard's token
endpoint, and exits if Shipyard refuses them. If Shipyard can't be reached then,
it starts anyway. It never reads or creates a CLI config file, so it runs on a
read-only filesystem, and the image runs it as a non-root user (uid 10001). Its own messages go to stderr; the tools' request logs,
which carry users' parameters, appear only with `--verbose`.

`GET /healthz` answers `ok` for load balancer checks.

### Deploying the hosted server

Shipyard runs it from the chart in `helm/charts/mcp-host`:

| Environment | MCP URL | Shipyard | Namespace |
|---|---|---|---|
| Production | `https://mcp.shipyard.build/mcp` | `https://shipyard.build` | `mcp-host` |
| QA | `https://mcp.qa.shipyard.build/mcp` | `https://qa.shipyard.build` | `qa-mcp-host` |

The MCP URL is every token's audience. Changing it later invalidates every
connection, so clients have to sign in again.

1. **Core first.** Set `OAUTH_MCP_RESOURCE` to the MCP URL, exactly, and
   `OAUTH_MCP_HOST_CLIENT_SECRET` to a secret generated in the secret store, a
   different one per environment. Both settings must be in place before the host
   starts: it checks them with core at startup and exits if core refuses them.
2. **DNS.** Point the host name at the cluster's ingress. cert-manager issues
   the certificate (`letsencrypt-production`) once the name resolves.
3. **Image**, from a release tag on a clean tree:

   ```bash
   git checkout v1.11.0
   make mcp-host.push
   ```

4. **Release**, with the kube context on the right cluster and the secret in
   `SHIPYARD_MCP_CLIENT_SECRET` (from the secret store, not typed into a shell
   history):

   ```bash
   make mcp-host.install.qa MCP_HOST_TAG=v1.11.0
   ```

   The secret goes to helm on stdin. `--atomic` rolls back a release whose pods
   don't become ready.
5. **Check it** from outside the cluster. A request without a token gets a 401
   pointing at the metadata, and the metadata names core:

   ```bash
   curl -si -X POST https://mcp.qa.shipyard.build/mcp | grep -i www-authenticate
   curl -s https://mcp.qa.shipyard.build/.well-known/oauth-protected-resource/mcp
   ```

What the chart sets, and why:

- **No sticky sessions.** The server is stateless, so any of its replicas
  (two by default, spread over nodes, with a disruption budget) serves any request.
- **Streaming.** nginx doesn't buffer requests or responses, and waits 120
  seconds for the server, longer than the server's own 90 second write timeout.
- **Locked down.** The image runs as uid 10001 on a read-only filesystem with
  no Kubernetes API token. The secret reaches it from a Secret, and a new
  secret restarts the pods. To use a Secret managed elsewhere, set
  `mcp.existingSecret`; then restart the pods yourself when it changes.
- **Logs.** `mcp.verbose` adds the tools' request logs, which carry users'
  parameters. Leave it off in production.

**Rotating the secret.** Core accepts one host secret at a time, so change
core's `OAUTH_MCP_HOST_CLIENT_SECRET` and the host's release together. Until
both match, a call that needs a new exchanged token fails with a retry hint;
tokens the host already holds keep working until they expire.

**Rolling back.** `helm rollback mcp-host -n <namespace>` returns to the
previous image and settings.

## Updating, uninstalling and rolling back

### Updating

```bash
shipyard upgrade
```

From 1.10.0, `shipyard upgrade` installs the latest release, through Homebrew if
that's how you installed. From 1.9.0 or earlier, upgrade once the way you
installed; see [Upgrading from 1.9.0 or earlier](../README.md#upgrading-from-190-or-earlier).
Then restart the MCP server: restart the client, or use its MCP panel
(`/mcp` in Claude Code). In clients that show the server's instructions, such as
Claude Code, an assistant connected to an out-of-date CLI is told so when the
session starts, once the CLI's daily update check has seen the new release. It
isn't told when `CI` is set or update checks are off. Codex doesn't show them.

If the upgrade changed the binary's path, update the `command` in the client's
config. Cursor asks you to approve the server again, and a per-app firewall needs
a fresh approval too.

### Uninstalling

Remove the server from the client:

| Client | Command or file |
|---|---|
| Claude Code | `claude mcp remove shipyard` (add `--scope user` if you added it there) |
| Claude Desktop | Delete the `shipyard` entry from `claude_desktop_config.json`, then restart |
| Cursor | Delete the `shipyard` entry from `~/.cursor/mcp.json` or `.cursor/mcp.json`, and any `Mcp(shipyard:…)` rules from `cli.json` |
| Codex | `codex mcp remove shipyard` |
| Kiro | Delete the `shipyard` entry from `~/.kiro/settings/mcp.json` or `.kiro/settings/mcp.json`, or set `"disabled": true` to keep it for later |

To remove the CLI and its token as well: `brew uninstall shipyard` for a Homebrew
install, otherwise delete the binary (`which shipyard`). There is no logout
command; delete the token from `~/.shipyard/config.yaml` (the `api_token` line, or
the whole file). An org API key you set in a client's `env` block goes away with
that entry; rotate it if it may have leaked.

### Rolling back

To go back to an earlier release, download it from the
[releases page](https://github.com/shipyard/shipyard-cli/releases), check it
against that release's `checksums.txt`, and point the client's `command` at the
binary's absolute path:

```bash
VERSION=1.9.0; OS=darwin; ARCH=arm64   # OS: darwin or linux; ARCH: arm64 or amd64
curl -fsSLO https://github.com/shipyard/shipyard-cli/releases/download/v$VERSION/shipyard-$OS-$ARCH
curl -fsSLO https://github.com/shipyard/shipyard-cli/releases/download/v$VERSION/checksums.txt
grep " shipyard-$OS-$ARCH\$" checksums.txt | shasum -a 256 -c -
chmod +x shipyard-$OS-$ARCH
```

On Windows, download `shipyard-windows-amd64.exe` (or `-arm64.exe`) and compare
its SHA-256 (`certutil -hashfile <file> SHA256`) with the line in `checksums.txt`.

Restart the client afterwards. Rolling back below 1.10.0 removes the tool hints,
so the Codex `writes` setting above stops offering the read tools without asking.

## Troubleshooting

- **The client can't start the server, or says `shipyard` isn't found.** The
  client doesn't see your shell's `PATH`. Use the absolute path from
  `which shipyard` as the command.
- **The server won't start: "Connection closed" or `unknown command "mcp"`.**
  The `shipyard` the client runs predates the MCP server (before 1.8.0).
  Claude Code shows only "Connection closed"; Cursor shows the
  `unknown command "mcp" for "shipyard"` it printed. Run `which -a shipyard` to
  list every copy on the `PATH`; the first one wins. Upgrade it (`brew upgrade
  shipyard` for Homebrew), remove the stale copy, or point the client at the
  newer binary's absolute path.
- **The `verify` prompt or newer tools are missing.** An older CLI earlier on the
  `PATH`, often a Homebrew install, is answering instead. Check
  `shipyard --version` in the client's environment and upgrade with
  `shipyard upgrade`. When the CLI's last update check found a newer release,
  the server tells the assistant so on startup, and the assistant can suggest
  the upgrade. Terminal commands run that check, and so does the server, in the
  background at most once a day, for the next session. `SHIPYARD_NO_UPDATE_CHECK`,
  `CI`, or `update_check: false` turns both off.
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
- **The Kiro IDE won't list or install powers: `Invalid installed.json schema`.**
  The Powers panel can't list, install or uninstall any power; the `mcp.json`
  setup above is unaffected. The Kiro CLI and the Kiro IDE share
  `~/.kiro/powers/installed.json` but write it in different formats: after
  `kiro-cli` installs or uninstalls a power, the IDE rejects the file (seen with
  Kiro CLI 2.28.0 through 2.30.0 and Kiro 1.2.56). Move the file aside and
  install again; the IDE recreates it. Powers it listed need reinstalling: do
  that from the IDE's Powers panel, since installing or uninstalling with
  `kiro-cli` breaks the file again. This is a Kiro bug, tracked as
  [kirodotdev/Kiro#11924](https://github.com/kirodotdev/Kiro/issues/11924);
  remove this entry once a Kiro IDE release accepts the CLI's file (the issue
  may stay open for a CLI-side fix).

  ```bash
  mv ~/.kiro/powers/installed.json ~/.kiro/powers/installed.json.$(date +%s).bak
  ```
