# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

- **Build executable**: `make` or `make build` - Creates binary in `bin/shipyard`
- **Run tests**: `make test` - Runs Go tests with coverage and golangci-lint
- **Build docker image**: `make build-docker`
- **Run tests individually**: `go test ./pkg/client -v` or `go test ./...`
- **Run linter**: `golangci-lint run`

## Project Architecture

This is the Shipyard CLI, a Go application built with Cobra for managing ephemeral environments on the Shipyard platform.

### Core Structure

- **main.go**: Entry point with panic handling, delegates to commands.Execute()
- **commands/**: Contains all CLI command implementations using Cobra
  - **root.go**: Root command setup, config initialization, command registration
  - **env/**: Environment management commands (stop, restart, cancel, rebuild, etc.)
  - **k8s/**: Kubernetes operations (exec, logs, port-forward)
  - **volumes/**: Volume operations (create, reset, upload, snapshots)
  - **telepresence/**: Telepresence connectivity

### Key Packages

- **pkg/client/**: HTTP client wrapper with org lookup functionality
- **pkg/requests/**: HTTP request handling and API communication
- **pkg/types/**: Data type definitions and parsing utilities
- **config/**: Configuration management (YAML-based, defaults to ~/.shipyard/config.yaml)
- **auth/**: Authentication handling
- **constants/**: Application constants
- **logging/**: Logging configuration

### Configuration

- Uses Viper for configuration management
- Default config: `$HOME/.shipyard/config.yaml`
- Environment variables prefixed with `SHIPYARD_` (e.g., `SHIPYARD_API_TOKEN`)
- CLI flags override config values

### Client Architecture

The application uses a dependency injection pattern:
1. `requests.New()` creates HTTP requester
2. `client.New(requester, orgLookupFn)` creates API client
3. Commands receive the client and use it for API calls

### Authentication

- Login via `shipyard login` (browser-based OAuth)
- Manual token via `SHIPYARD_API_TOKEN` env var or config file
- Token stored in config file after login

### MCP Integration

- **MCP Server**: `shipyard mcp serve` - Starts Model Context Protocol server for AI assistant integration
- **Location**: `pkg/mcp/` - Contains MCP server implementation with tools, resources, and transport layers
- **Tools**: Environment ops, service management, logging, volume operations
- **Transport**: stdio-based communication for AI assistants

### Testing

- Unit tests in `*_test.go` files
- Integration tests in `tests/` directory with mock server
- Test coverage tracking enabled

## Skill routing

When the user's request matches an available skill, ALWAYS invoke it using the Skill
tool as your FIRST action. Do NOT answer directly, do NOT use other tools first.
The skill has specialized workflows that produce better results than ad-hoc answers.

Key routing rules:
- Product ideas, "is this worth building", brainstorming → invoke office-hours
- Bugs, errors, "why is this broken", 500 errors → invoke investigate
- Ship, deploy, push, create PR → invoke ship
- QA, test the site, find bugs → invoke qa
- Code review, check my diff → invoke review
- Update docs after shipping → invoke document-release
- Weekly retro → invoke retro
- Design system, brand → invoke design-consultation
- Visual audit, design polish → invoke design-review
- Architecture review → invoke plan-eng-review
- Save progress, checkpoint, resume → invoke checkpoint
- Code quality, health check → invoke health
