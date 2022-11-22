# The Shipyard CLI
A tool to manage Ephemeral Environments on the Shipyard platform.

#### Installation:

TODO

#### Before you Begin:
You will need to set up your API token as an environment variable `SHIPYARD_API_TOKEN`
You can get your token by going to [your profile page](https://shipyard.build/profile). Get in touch with us if you would like to enable API access for your org.

#### Basic Usage:
- **List all environments:**
    ```bash
    shipyard get environments
    ```
    **Available flags:**
    | Name | Description | Type | Default Value |
    | -| - | - | - |
    | `branch` | Filter by branch name | string | |
    | `deleted`| Return deleted environments | boolean | `false` |
    | `json` | Print the *complete* JSON output  | boolean | `false` |
    | `name` | Filter by name of the application | string | |
    | `org-name` | Filter by org name, if you are part of multiple orgs | string | `your default org` |
    | `page` | Page number requested | int | `1` |
    | `page-size` | Page size requested | int | `20` |
    | `pull-request-number` | Filter by pull request number | string |  |
    | `repo-name` | Filter by repo name | string |  |
    
    **Examples:**
    - List all environments for a specific repo `flask-backend` and branch `main`:
        ```bash
        shipyard get environments --repo-name flask-backend --branch main
        ```
    - List all deleted environments:
        ```bash
        shipyard get environments --deleted
        ```
    
- **Get details for a specifc environment by it's UUID:**
    ```bash
    shipyard get environment {environment_uuid}
    ```
    **Available flags:**
    | Name | Description | Type | Default Value |
    | -| - | - | - |
    | `json` | Print the *complete* JSON output  | boolean | `false` |
    | `org-name` | Filter by org name, if you are part of multiple orgs | string | `your default org` |
 
- **Get all services and exposed ports for an environment:**
    ```bash
    shipyard get services --env {environment_uuid} 
    ```
- **Get all orgs you are part of:**
    ```bash
    shipyard get orgs
    ```
- **Stop a *running* environment:**
    ```bash
    shipyard stop environment {environment_uuid}
    ```
- **Restart a *stopped* environment:**
    ```bash
    shipyard restart environment {environment_uuid}
    ```
- **Cancel *on-going build* for an environment:**
    ```bash
    shipyard cancel environment {environment_uuid}
    ```
- **Rebuild an environment:**
    ```bash
    shipyard rebuild environment {environment_uuid}
    ```
- **Revive a *deleted* environment:**
    ```bash
    shipyard revive environment {environment_uuid}
    ```
- **Exec into a *running* environment's service:**
   Execute any command with any arguments and flags in a given service for a **running** environment. Pass any command arguments after a double slash.
    ```bash
    shipyard exec --env {environment_uuid} --service {service_name} -- bash
    ```
- **Port forward a *running* environment's service's port:**
    ```bash
    shipyard port-forward --env {environment_uuid} --service {service_name} --ports 80:80
    ```
- **Get logs for a *running* environment's service:**
    ```bash
    shipyard logs --env {environment_uuid} --service {service_name}
    ```
    **Available flags:**
    | Name | Description | Type | Default Value |
    | -| - | - | - |
    | `follow` | Follow the logs output | boolean | `false`|
    | `tail`| # of recent log lines to show | int | `3000` |
#### Building executable from code:
You can make an executable by running the make command:
```bash
make build
```
To run this new executable:
```bash
./shipyard
```
