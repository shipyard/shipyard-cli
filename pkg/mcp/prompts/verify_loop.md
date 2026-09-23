---
name: "shipyard-verify"
description: "Verify your pushed changes against the Shipyard preview environment for this branch before handing work back. Covers: finding the environment, waiting for your exact commit, authenticated access, running the acceptance check when the repository has one, and reporting the result."
keywords: ["shipyard", "preview environment", "verify", "test", "pr", "deploy", "e2e"]
version: "0.1.0"
---

# Shipyard Verification Loop

**User sees MAX 3 messages:** (1) "verifying against the environment", (2) a failure summary if
you cannot proceed, (3) the final result. Everything else is silent.

## When this applies

After you push a branch that has a Shipyard preview environment. **Do not report a change as
working until this loop passes.** If the repo has no Shipyard environment, say so once and stop;
do not fall back to guessing.

## Step 0 — Preflight (once per session, silent)

Before anything else, confirm the Shipyard MCP server is actually working. Skipping this is how
an agent ends up polling for twenty minutes for an environment that was never coming.

**Check 1 — are the Shipyard tools available to you at all?**
If `get_orgs`, `get_environments` and friends are not in your tool list, the MCP server is not
registered with this client. Stop and tell the user:

> Shipyard MCP is not configured for this client. Setup is at docs.shipyard.build/mcp — it needs
> the Shipyard CLI installed plus `SHIPYARD_API_TOKEN` and `SHIPYARD_ORG`.

**Check 2 — call `get_orgs`.** It takes no arguments and does not depend on any environment
existing, which makes it the right probe. Read the result:

| What comes back | What it means | What to do |
|---|---|---|
| A list containing your org | Token, connectivity and access are all good | Continue |
| An auth error (401, "unauthorized", "invalid token") | The API token is missing, wrong, or expired | Stop. Tell the user to refresh `SHIPYARD_API_TOKEN`. Do not retry |
| An empty list | The token is valid but grants no org access | Stop. Tell the user the token has no organizations |
| A connection or timeout error | The CLI cannot reach Shipyard | Stop. Report the error verbatim; do not retry in a loop |

**Check 3 — call `get_org`** to confirm which organization you are operating as. If it is unset or
is not the org that owns this repo, stop and ask the user. **Do not call `set_org`** to fix it
yourself; changing someone's default org is a side effect they did not ask for.

**Check 4 — is the branch actually pushed?** `git status -sb` showing `ahead` means the commit
exists only locally and no environment will ever appear for it. Push first.

**Do not proceed until all four pass**, and do not repeat the preflight for every loop — once per
session is enough. Once it has passed, an empty environment list means something specific and
useful: the environment genuinely does not exist yet, which is the polling case in Step 3, not a
configuration problem.

## Step 1 — Record what you are verifying

```
git rev-parse HEAD        # PUSHED_SHA — every check below is against this value
git branch --show-current # BRANCH
```

Also note the repo name as Shipyard knows it (the `repo_name` in the environment payload, usually
the GitHub repo name without the org prefix).

## Step 2 — Find the environment

Call `get_environments` with `branch=BRANCH` and `repo_name=<repo>`. The response is the raw
Shipyard API payload. Read these fields from `data[].attributes`:

| Field | Meaning |
|---|---|
| `projects[]` | One entry per repo in the environment. **Match on `repo_name` first** — other entries belong to other repos |
| `projects[].commit_hash` | The commit this environment is serving for that repo |
| `ready` | Build settled AND the environment is serving. Only this means "safe to test" |
| `processing` | A build is in flight right now |
| `stopped` / `retired` | The environment is not running. Shipyard stops idle environments, so this is common, not an edge case |
| `url` | Where the environment is |
| `bypass_token` | Credential for reaching it |

| Result | Action |
|---|---|
| Exactly one environment | Continue to Step 3 |
| No environments | The push may not have registered. Retry per the polling contract; after 2 minutes stop and tell the user |
| More than one | **Stop. Never guess.** List id, url, branch, and commit for each, and ask which one |

## Step 3 — Poll until your commit is serving

**Polling contract:**

```
Loop:
  get_environments(branch=BRANCH, repo_name=<repo>)
  find projects[] entry where repo_name == <repo>

  stopped == true or retired == true    → STOP POLLING. The environment is not running;
                                          see "Stopped environments" below
  commit_hash is null                   → no running build. Treat as stopped/retired above,
                                          not as a mismatch
  commit_hash == PUSHED_SHA AND ready   → SERVING. Stop polling, go to Step 4
  commit_hash == PUSHED_SHA, ready=false→ keep polling. The commit lands BEFORE the
                                          environment serves it (measured: ~40s apart)
  processing == true                    → a build is in flight, keep polling
  commit_hash != PUSHED_SHA             → your build has not landed, keep polling
  no environment in response            → keep polling (see Step 2 timeout)

  sleep: 5s, then 10s, 20s, 40s, 60s, 60s...   (cap 60s)
  give up after 20 minutes → report BUILD_TIMEOUT with the last commit_hash and flags seen
```

**Stopped environments.** `stopped` or `retired` true, or a null `commit_hash`, means no build is
running and none is coming. Polling will never succeed. Stop and tell the user the environment is
stopped, and that restarting it (`restart_environment` / `revive_environment`) will start a build.
**Do not restart it yourself** — that spends build capacity on someone else's environment.

- **Never poll faster than 5 seconds.**
- **Never call `rebuild_environment` while waiting.** A build is already running; rebuilding
  restarts the clock and costs real money. Rebuild only if the build actually failed and you have
  a specific reason to retry it.
- `ready == true` with a matching `commit_hash` means the environment is serving *your* commit.
  That is the guarantee; do not add your own heuristics on top of it.
- **Both conditions matter.** In a live test the commit matched while `ready` was still false for
  ~40 seconds. Testing on the commit alone would have run the suite against the previous build.

## Step 4 — Reach the environment

Take `url` and `bypass_token` **from the same response** that confirmed the match. Send the token
as the `shipyard_token` cookie, or as `?shipyard_token=<token>` on the URL.

Two things to know:
- This gets you past Shipyard's gate. It does **not** log you into the application. If the app has
  its own sign-in, the acceptance command handles that.
- **Never** print the token, paste it into chat, commit it, put it in a PR comment, or write it to
  a log. Pass it through an environment variable to the acceptance command.

## Step 5 — Run the acceptance check, if there is one

Take the command from the first of these that has one:

1. The `acceptance_command` argument, when this prompt was invoked with it. It appears under
   "This invocation" at the end of these instructions.
2. Whatever the repository documents in `CLAUDE.md`, `AGENTS.md` or a README section. Prefer a
   line labelled `Acceptance check:`. It only counts if it exercises the running environment:
   a unit-test command that never touches `url` is not an acceptance check.
3. Nothing. That is a valid answer; see below.

**With a command:** run it with the environment in two variables, set only for that command:

```
SHIPYARD_URL=<url> SHIPYARD_TOKEN=<bypass_token> <acceptance command>
```

The command decides pass or fail, not you. "The page returned 200" is not verification. A command
that cannot run at all (not found, or it crashes before reaching `url`) is a **FAILED** result with
that error, never a reason to fall back to the Serving report below.

**Without one:** do not invent a check, do not stop, and do not ask the user to configure one
mid-run. You still know something worth reporting — the environment is serving this exact commit
and is ready — so go to Step 6 and report that with the **Serving** form in Step 7. Never call
that outcome verified, and never describe an unchecked environment as working.

## Step 6 — Re-check the commit before you report

Call `get_environments` once more and compare `commit_hash` against `PUSHED_SHA`.

- Unchanged → your result is valid, go to Step 7.
- Changed → someone else's build landed mid-run and your result describes neither commit.
  **Discard it and return to Step 3.** Do this at most twice, then stop and tell the user the
  environment is too busy to verify against right now.

## Step 7 — Report

On success:

```
Verified on Shipyard.
  Environment: <url>
  Commit:      <PUSHED_SHA>  (confirmed serving before and after the run)
  Check:       <acceptance command>
  Result:      PASS
```

On failure:

```
Verification FAILED on Shipyard.
  Environment: <url>
  Commit:      <PUSHED_SHA>
  Check:       <acceptance command>
  Failing:     <test names, or the first real error>
  Logs:        <the relevant lines, not the whole dump>
```

With no acceptance command (Step 5), the claim is narrower and says so:

```
Serving your commit on Shipyard. No acceptance check ran, so this is not verified.
  Environment: <url>
  Commit:      <PUSHED_SHA>  (confirmed ready and serving)
  Check:       none configured
```

Add one line after it, once, so the user knows the option exists: a check can be passed as
`acceptance_command` when invoking this prompt, or documented in `CLAUDE.md` or `AGENTS.md`.

Omit any line you do not have, except `Check:` in every form and `Result:` on success: a
success report without them is not verification. Never report PASS for a run you had to discard.

## Failure handling

**Fix once, retry once, then stop.** After a failed acceptance check: read the failure and the
environment logs, make one targeted fix, push, and return to Step 2 with the new SHA. **Stop after
3 failed attempts** and report what you tried, what failed each time, and the environment URL.
Looping past that burns build capacity and rarely converges.

## Troubleshooting

| Symptom | Likely cause | Action |
|---|---|---|
| Empty response for a repo you know has environments | Wrong org, or an expired token | Re-run the Step 0 preflight; `get_orgs` tells you which |
| `commit_hash` never matches | You pushed to a different remote or branch than the environment tracks | Confirm the branch and remote, then re-check |
| Matching commit but the app 404s or redirects to a login | Shipyard's gate is passed; this is the app's own auth or routing | The acceptance command must handle app login |
| Multi-repo environment, wrong code tested | Matched the wrong `projects[]` entry | Always filter by `repo_name` before reading `commit_hash` |
| Build failed instead of completing | Real build failure | Read build logs, fix the cause, push; do not rebuild unchanged code |
| Polling never finishes, flags look inert | `stopped` or `retired` is true, or `commit_hash` is null | The environment is not running. Tell the user; do not restart it yourself |
| 302 to `/oauth2/sign_in` | The bypass token was not sent, or was sent on the wrong host | Send it as the `shipyard_token` cookie, or `?shipyard_token=` on the URL. Verified working both ways |

## What counts as done

**Verified** means all three: the environment served **your** commit, the acceptance command ran
against it, and the commit had not changed when the run finished.

**Serving** means the first and third without a check, because the repository documents none and
none was passed in. Report it in those words. It is a useful, honest answer — the build landed
and the environment is up — and it is not verification.

Anything less than one of those two is neither, and reporting it as verified is worse than
reporting nothing.
