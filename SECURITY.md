# Security policy

## Reporting a vulnerability

Email **security@shipyard.build**. Please don't open a public GitHub issue for a security problem.

This covers the `shipyard` CLI and its MCP server (`shipyard mcp serve`) in this repository, and the Shipyard platform
they talk to. Include what you can of:

- the version (`shipyard --version`) and your OS;
- the steps or a proof of concept that shows the issue;
- the impact as you understand it.

Don't include real API tokens or other credentials in the report; describe where one appears instead.

We typically respond within one business day. We won't take legal action against anyone reporting in good faith.
Please act in good faith toward our users' privacy and data while you investigate. Our full disclosure policy is at
<https://shipyard.build/security/>.

## Supported versions

Security fixes ship in the latest release. Upgrade with `shipyard upgrade` (from 1.10.0) or the way you installed;
see [Upgrading](README.md#upgrading).

## Handling credentials

- The CLI and MCP server read your token from `SHIPYARD_API_TOKEN` or `~/.shipyard/config.yaml`, which the CLI creates
  with mode 0600 (if you created it yourself, run `chmod 600 ~/.shipyard/config.yaml`). Keep it out of repositories and
  out of project-level client config files; see
  [Permissions and least privilege](docs/mcp.md#permissions-and-least-privilege).
- If a token may have leaked, regenerate it from your Shipyard profile page (personal token) or your org's settings
  (org API key).
- Release binaries are listed with their SHA-256 in each release's `checksums.txt`.
