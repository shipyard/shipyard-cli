Shipyard runs ephemeral preview environments, typically one per branch or pull request.

## Verifying a change against its environment

After pushing a branch that has a Shipyard environment, verify the change against that
environment before reporting it as working. Record the pushed SHA (`git rev-parse HEAD`), call
`get_environments` with `branch` and `repo_name`, and in the response match `projects[]` on
`repo_name` before reading anything from it:

- `commit_hash` equals your pushed SHA AND `ready` is true — the environment is serving your
  commit. Use `url` and `bypass_token` from that same response; send the token as the
  `shipyard_token` cookie, never on the URL.
- `commit_hash` matches but `ready` is false — keep polling. The commit lands roughly 40 seconds
  before the environment serves it, so matching on the commit alone tests the previous build.
- `stopped` or `retired` is true, or `commit_hash` is null while `processing` is false — the
  environment is not running and will not become ready on its own. Stop and tell the user; do not
  restart it yourself.
- `processing` is true — a build is in flight. Poll no faster than every 5 seconds, back off, and
  never call `rebuild_environment` while waiting: a build is already running and rebuilding
  restarts it.

After the check runs, call `get_environments` once more. If `commit_hash` changed, a rebuild
landed mid-run and the result describes neither commit: discard it and re-run.

Before checking, propose a test plan and wait for the user's approval (unless auto-approved).
Report a change as verified only when, on the commit you pushed, every approved item passed and
every changed behavior is covered by a test or check with a quoted assertion. Checks
you add can use any tool (e2e, API tests, `curl`, `exec_service`) but must be reproducible, and
for a fix or changed behavior must fail on the base branch's environment. Nothing from a container
you edited in place counts. Otherwise report what is true: passed but not covered, observed,
serving only, or failed.

## Notes

- The `shipyard_verify` prompt carries the full loop, including preflight and failure handling.
- `get_orgs` takes no arguments and does not depend on any environment existing, which makes it
  the right probe when you need to tell a broken setup from an environment that does not exist yet.
- Never print a `bypass_token`, or paste it into chat, a commit, or a pull request comment.
