# The Shipyard CLI

A tool to manage Ephemeral Environments on the Shipyard platform.

## Installation

- **Mac / Linux**
    ```
    curl https://shipyard.sh/install.sh | bash
    ```
- **Windows**
    - Navigate to [releases page](https://github.com/shipyardbuild/shipyard-cli/releases) and download the executable.

## Before you begin

Set the environment variable `SHIPYARD_API_TOKEN` to your Shipyard API token.
You can get your token by going to [your profile page](https://shipyard.build/profile). Get in touch with us if you would like to enable API access for your org.

## Basic usage

### List all environments
```bash
shipyard get environments
```

Available flags:

| Name | Description | Type | Default Value |
| -| - | - | - |
| branch | Filter by branch name | string | |
| deleted| Return deleted environments | boolean | false |
| json | Print the complete JSON output  | boolean | false |
| name | Filter by name of the application | string | |
| org-name | Filter by org name, if you are part of multiple orgs | string | your default org |
| page | Page number requested | int | 1 |
| page-size | Page size requested | int | 20 |
| pull-request-number | Filter by pull request number | string |  |
| repo-name | Filter by repo name | string |  |

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

| Name | Description | Type | Default Value |
| -| - | - | - |
| json | Print the complete JSON output  | boolean | false |
| org-name | Filter by org name, if you are part of multiple orgs | string | your default org |

### Get all services and exposed ports for an environment
```bash
shipyard get services --env {environment_uuid}
```

### Get all orgs you are a member of
```bash
shipyard get orgs
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

### Exec into a running environment's service
   Execute any command with any arguments and flags in a given service for a **running** environment. Pass any command arguments after a double slash.
```bash
shipyard exec --env {environment_uuid} --service {service_name} -- bash
```

### Port forward a running environment's service's port
```bash
shipyard port-forward --env {environment_uuid} --service {service_name} --ports 80:80
```

### Get logs for a running environment's service
```bash
shipyard logs --env {environment_uuid} --service {service_name}
```

Available flags:

| Name | Description | Type | Default Value |
| -| - | - | - |
| follow | Follow the logs output | boolean | false|
| tail| # of recent log lines to show | int | 3000 |


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
This script depends on the `bash-completion` package. If it is not installed already, you can install it via your OS's package manager.
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
If shell completion is not already enabled in your environment, you will need to enable it. You can execute the following once:
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
