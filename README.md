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
| org-name | Filter by org name, if you are part of multiple orgs | string  | your default org |

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

#### Limited Tools
These tools return help text directing users to use CLI commands instead:
- `exec_service` - Execute commands in service containers
- `port_forward` - Port forward services to local machine
- `telepresence_connect` - Connect to telepresence

### Adding to Claude

With API token and org name:
```bash
claude mcp add shipyard --env SHIPYARD_API_TOKEN=your-token-here --env SHIPYARD_ORG=your-org-name -- shipyard mcp serve
```

If already configured with CLI:
```bash
claude mcp add shipyard -- shipyard mcp serve
```
