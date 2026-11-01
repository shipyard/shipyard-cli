Shipyard runs preview environments, usually one per branch or PR.

## Adding an app

Call `plan_application` first and show the user the plan: branch, name, services, routes and the
env vars it asks for. Call `create_application` only after the user confirms it. Never put an env
var's value in chat; ask the user for it.

## Checking a change

After pushing a branch that has an environment, record the pushed SHA (`git rev-parse HEAD`), call
`get_environments` with `branch` and `repo_name`, and match `projects[]` on `repo_name`:

- `commit_hash` equals your pushed SHA AND `ready` is true: it serves your commit. Use `url`, and
  `bypass_token` when there is one, sent as the `shipyard_token` cookie, never on the URL; never
  print it or put it in chat, a commit or a PR.
- `commit_hash` matches but `ready` is false: keep polling, at most once per 5 seconds. The commit
  lands about 40 seconds before it is served.
- `processing` is true: a build is in flight; keep polling.
- A failed build: call `get_failure_details`, then `get_build_logs` for the failing service.
