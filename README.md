# The Shipyard CLI

A tool to manage Ephemeral Environments on the Shipyard platform.

## Installation

- **Linux and macOS**
    ```
    curl https://www.shipyard.sh/install.sh | bash
    ```
- **Windows**
  Navigate to the [releases page](https://github.com/shipyard/shipyard-cli/releases) and download the executable for
  Windows.

- **Homebrew**
    ```
    brew tap shipyard/tap
    brew install shipyard
    ```

## Login

Run `shipyard login` to initialize the CLI. This will prompt you to log in to Shipyard in the browser. The CLI will then
save your API token in a local config. You're ready to start running commands.

### Or Set Your Token Manually

Set your Shipyard API token as the value of the `SHIPYARD_API_TOKEN` environment variable.

You can get it by going to [your profile page](https://shipyard.build/profile).

You can get in touch with us at [support@shipyard.build](mailto:support@shipyard.build) if you would like to enable API
access for your org. If you have any other questions, feel free to join
our community [Slack](https://join.slack.com/t/shipyardcommunity/shared_invite/zt-x830cx39-BuiQKZwvhG7zGRTXAvojVQ).

```bash
shipyard set token
```

Alternatively, you can use a configuration file stored in `$HOME/.shipyard/config.yaml` by default.
When you run the CLI for the first time, it will create a default empty config that you can then edit.

You can also specify a non-default config path with the `--config {path}` flag added to any command.

Add any configuration values in your config and ensure the file follows YAML syntax.
For example:

```yaml
api_token: <your-token>
org: <your-non-default-org>
```

The values of your environment variables override their corresponding values in the config.

## Basic usage

### Get all orgs you are a member of

```bash
shipyard get orgs
```

### Set the global default org

```bash
shipyard set org {org-name}
```

### Get the currently configured org

```bash
shipyard get org
```

### List all environments

```bash
shipyard get environments
```

Available flags:

| Name                | Description                                          | Type    | Default Value    |
|---------------------|------------------------------------------------------|---------|------------------|
| branch              | Filter by branch name                                | string  |                  |
| deleted             | Return deleted environments                          | boolean | false            |
| json                | Print the complete JSON output                       | boolean | false            |
| name                | Filter by name of the application                    | string  |                  |
| org-name            | Filter by org name, if you are part of multiple orgs | string  | your default org |
| page                | Page number requested                                | int     | 1                |
| page-size           | Page size requested                                  | int     | 20               |
| pull-request-number | Filter by pull request number                        | string  |                  |
| repo-name           | Filter by repo name                                  | string  |                  |

**Examples:**

- List all environments running the repo `flask-backend` on branch `main`:

```bash
shipyard get environments --repo-name flask-backend --branch main
```

- List all deleted environments:

```bash
shipyard get environments --deleted
```

### Get details for a specifc environment by its UUID

```bash
shipyard get environment {environment_uuid}
```

Available flags:

| Name     | Description                                          | Type    | Default Value    |
|----------|------------------------------------------------------|---------|------------------|
| json     | Print the complete JSON output                       | boolean | false            |
| org      | Org of the environment, if you are part of multiple orgs | string  | your default org |
| bypass-token | Print only the environment's bypass token, for scripts | boolean | false |

`--bypass-token` lets a script use the token without anyone typing or printing it:

```bash
SHIPYARD_TOKEN=$(shipyard get environment {environment_uuid} --bypass-token) && \
  export SHIPYARD_TOKEN && curl -b "shipyard_token=$SHIPYARD_TOKEN" https://your-environment-url/
```

### Stop a running environment

```bash
shipyard stop environment {environment_uuid}
```

### Restart a stopped environment

```bash
shipyard restart environment {environment_uuid}
```

### Cancel ongoing build for an environment

```bash
shipyard cancel environment {environment_uuid}
```

### Rebuild an environment

```bash
shipyard rebuild environment {environment_uuid}
```

### Revive a deleted environment

```bash
shipyard revive environment {environment_uuid}
```

### Deploy a detached environment

Create a new, independent ("detached") environment by cloning an existing application build.
Requires detached environments to be enabled for your org.

```bash
shipyard detached deploy {application_build_uuid} --name my-detached-env
```

Override branches per-repo and control whether the detached environment rebuilds on new commits:

```bash
# Override the branch for a repo, and never rebuild on new commits
shipyard detached deploy {application_build_uuid} --name my-detached-env --branch web=feature-x --build-on-commit never

# Per-repo build-on-commit settings (always | inherit | never)
shipyard detached deploy {application_build_uuid} --build-on-commit-for web=always --build-on-commit-for api=never
```

### Get all services and exposed ports for an environment

```bash
shipyard get services --env {environment_uuid}
```

### Exec into a running environment's service

Execute any command with any arguments and flags in a given service for a **running** environment. Pass any command
arguments after a double slash.

```bash
shipyard exec --env {environment_uuid} --service {service_name} -- bash
```

### Port forward a running environment's service's port

```bash
shipyard port-forward --env {environment_uuid} --service {service_name} --ports {local_port}:{service_container_port}
```

### Get logs for a running environment's service

```bash
shipyard logs --env {environment_uuid} --service {service_name}
```

### Visit an environment

```bash
shipyard visit {environment_uuid}
```

Available flags:

| Name   | Description                   | Type    | Default Value |
|--------|-------------------------------|---------|---------------|
| follow | Follow the logs output        | boolean | false         |
| tail   | # of recent log lines to show | int     | 3000          |

## Work with volumes

### List all volumes in an environment

```bash
shipyard get volumes --env {environment_uuid}
```

### List all volume snapshots in an environment

```bash
shipyard get snapshots --env {environment_uuid}
```

### Reset a volume in an environment

```bash
shipyard reset volume --env {environment_uuid}
```

### Create a snapshot in an environment

```bash
shipyard create snapshot --env {environment_uuid}
```

### Load a volume snapshot in an environment

```bash
shipyard load snapshot --env {environment_uuid} --sequence-number {n}
```

### Upload a file to a volume in an environment

```bash
shipyard upload volume --env {environment_uuid} --volume {volume} --file {filepath.bz2}
```

### Connect to telepresence
```bash
shipyard telepresence connect --env {environment_uuid}
```

From there, you'll be able to communicate directly with all pods in the namespace.  You _may_ have to use the
namespace hostname to communicate with services, which you can get via `telepresence status` under the Namespace field.  For example, to communicate with redis, you'd use redis.shipyard-app-build-{uuid}


## Build executable from code:

You can make an executable by running the following command:

```bash
make
```

To run this new executable:

```bash
./shipyard
```

## Enable Autocompletion

### Bash

This script depends on the `bash-completion` package. If it is not installed already, you can install it via your OS's
package manager.
To load completions in your current shell session:

```
source <(shipyard completion bash)
```

To load completions for every new session, execute the following once.

On Linux:

```
shipyard completion bash > /etc/bash_completion.d/shipyard
```

On macOS:

```
shipyard completion bash > $(brew --prefix)/etc/bash_completion.d/shipyard
```

### Zsh

If shell completion is not already enabled in your environment, you will need to enable it. You can execute the
following once:

```
echo "autoload -U compinit; compinit" >> ~/.zshrc
```

To load completions in your current shell session:

```
source <(shipyard completion zsh); compdef _shipyard shipyard
```

To load completions for every new session, execute the following once.

On Linux:

```
shipyard completion zsh > "${fpath[1]}/_shipyard"
```

On macOS:

```
shipyard completion zsh > $(brew --prefix)/share/zsh/site-functions/_shipyard
```

You will need to start a new shell for this setup to take effect.

### Fish

To load completions in your current shell session:

```
$ shipyard completion fish | source
```

To load completions for each session, execute once:

```
shipyard completion fish > ~/.config/fish/completions/shipyard.fish
```

### PowerShell

To load completions in your current shell session:

```
shipyard completion powershell | Out-String | Invoke-Expression
```

To load completions for every new session, run:

```
shipyard completion powershell > shipyard.ps1
```

and source this file from your PowerShell profile.

## Model Context Protocol (MCP) Integration

The Shipyard CLI provides an MCP server for AI assistant integration. This allows AI assistants like Claude to manage Shipyard environments directly.

### Supported MCP Tools

#### Environment Management (7 tools)
- `get_environments` - List environments with filtering
- `get_environment` - Get specific environment details
- `stop_environment` - Stop a running environment
- `restart_environment` - Restart a stopped environment
- `rebuild_environment` - Rebuild with latest commit
- `cancel_environment` - Cancel environment's latest build
- `revive_environment` - Revive a deleted environment

#### Service Management (2 tools)
- `get_services` - List services in an environment
- `get_logs` - Get logs from a service

#### Volume Management (5 tools)
- `get_volumes` - List volumes in an environment
- `reset_volume` - Reset volume to initial state
- `get_snapshots` - List volume snapshots
- `create_snapshot` - Create volume snapshot
- `load_snapshot` - Load volume snapshot

#### Organization Management (3 tools)
- `get_orgs` - List all organizations
- `get_org` - Get current default organization
- `set_org` - Set default organization

#### Running commands in a container (opt-in)
- `exec_service` - Run a non-interactive command in a service container

Disabled by default, because it is the only tool here that runs arbitrary code
inside a running environment. Turn it on per machine:

```yaml
# ~/.shipyard/config.yaml
mcp:
  allow_exec: true
```

Or in the MCP client's own environment, which is usually easier since that is
where the rest of the server's settings live:

```
SHIPYARD_MCP_ALLOW_EXEC=true
```

Restart the client afterwards. While it is off, `exec_service` explains how to
enable it and hands back the equivalent `shipyard exec` command.

Once on, it returns stdout, stderr and the exit code. A command that exits
non-zero is a result, not an error. There is no terminal and no stdin, so
interactive programs (`bash`, `vim`, `psql` without `-c`) will not work — use
`shipyard exec` for those. After 60 seconds the tool closes the stream and
returns what the command printed so far, with a null `exit_code`. Closing the
stream does not kill the process in the container: one that keeps writing dies
on the closed pipe, one that never writes again runs until it exits. Each stream
is truncated past 64KB, with `truncated: true` in the response when that happens.

#### Limited Tools
These tools return help text directing users to use CLI commands instead:
- `port_forward` - Port forward services to local machine
- `telepresence_connect` - Connect to telepresence

Both need a connection that outlives a single request, so the CLI is the right
place for them.

### Adding to Claude

With API token and org name:
```bash
claude mcp add shipyard --env SHIPYARD_API_TOKEN=your-token-here --env SHIPYARD_ORG=your-org-name -- shipyard mcp serve
```

If already configured with CLI:
```bash
claude mcp add shipyard -- shipyard mcp serve
```

### Adding to Cursor

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

Drop the `env` block if the CLI is already configured: the server reads the same
`~/.shipyard/config.yaml` the CLI does. Reload the window, then check
Settings → MCP, where Shipyard should list its tools and the `verify`
prompt. `shipyard` has to be on the `PATH` Cursor itself sees; if it is not, use
its absolute path (`which shipyard`) as `command`.

### Adding to Codex CLI

Edit `~/.codex/config.toml` and add:

```
[mcp_servers.shipyard]
command = "shipyard"
args = ["mcp", "serve"]
env = { "SHIPYARD_API_TOKEN" = "your-token-here", "SHIPYARD_ORG" = "your-org-name" }
```

### Prompts

- `verify` - Verify a pushed change against its Shipyard environment:
  wait for the build of the pushed SHA, then check the running environment
  before reporting the change as working.

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
the CLI to be logged in in the agent's shell, not only in the MCP client's
`env` block. If it isn't, the agent types the token into its commands and says
so in the report; run `shipyard login` to avoid that.

#### The acceptance check

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

#### The test plan

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

#### Checking the change itself

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

#### Fixing without a rebuild per attempt

When `exec_service` is enabled (`mcp.allow_exec`) and the service runs a dev
server that reloads code, the agent can try a fix by writing changed files into
the running container, rerun its checks in seconds, and push once they pass. It
does this only after confirming the container serves the repository's files (by
comparing hashes) and runs a known reloading process. Results from an edited
container are never the verdict: after the push, the environment rebuilds and
the checks run again on the clean commit. If the agent stops without pushing, it
restores the files it changed.

### Troubleshooting

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
