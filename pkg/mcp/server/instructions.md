Shipyard runs ephemeral preview environments, usually one per branch or pull request.

## Verifying a change

Before reporting a pushed change as working, verify it against its environment. Record the
pushed SHA (`git rev-parse HEAD`), call `get_environments` with `branch` and `repo_name`, and
match `projects[]` on `repo_name` before reading anything:

- `commit_hash` equals your pushed SHA AND `ready` is true — it serves your commit. Use
  `url` and `bypass_token` from that response; send the token as the `shipyard_token` cookie,
  never on the URL, and never print or paste it anywhere.
- `commit_hash` matches but `ready` is false — keep polling: the commit lands about 40 seconds
  before it is served, so matching the commit alone tests the previous build.
- `stopped` or `retired` is true, or `commit_hash` is null while `processing` is false — it will
  not become ready on its own. The `verify` prompt says when you may start it; never start
  another branch's.
- `processing` is true — a build is in flight. Poll at most every 5 seconds, back off, and never
  call `rebuild_environment` while waiting: it restarts the build.

After the check, call `get_environments` again: if `commit_hash` changed, a rebuild landed
mid-run; discard the result and re-run.

Before checking, propose a test plan and wait for approval (unless auto-approved).
Report a change as verified only when, on the commit you pushed, every approved item passed and
every changed behavior is covered by a test or check with a quoted assertion. Checks
you add can use any tool (e2e, API tests, `curl`, `exec_service`) but must be reproducible and,
for a fix or changed behavior, fail on the base branch's environment. Nothing from a container
you edited in place counts. Otherwise report what is true: passed but not covered, observed,
serving only, or failed.

The `verify` prompt has the full loop, with preflight and failure handling.
