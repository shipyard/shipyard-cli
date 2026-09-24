---
name: "shipyard-verify"
description: "Verify your pushed changes against the Shipyard preview environment for this branch before handing work back. Covers: finding the environment, waiting for your exact commit, authenticated access, running the acceptance check when the repository has one, checking that the change itself is covered, and reporting the result."
keywords: ["shipyard", "preview environment", "verify", "test", "pr", "deploy", "e2e"]
version: "0.3.0"
---

# Shipyard Verification Loop

**User sees MAX 3 messages:** (1) "verifying against the environment", (2) the test plan, which
waits for their answer (Step 2b), or a failure summary if you cannot proceed, (3) the final
result. Everything else is silent. With auto-approve, the plan goes into the final result instead.

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

`BASE` is the name of the branch this change will merge into: the pull request's target
(`gh pr view <BRANCH> --json baseRefName -q .baseRefName`), else the remote's default branch
(`git symbolic-ref --short refs/remotes/origin/HEAD | sed 's@^origin/@@'`). It is a bare name
such as `main`, never `refs/remotes/origin/main`. Fetch it (`git fetch origin <BASE>`) and compare
against `origin/<BASE>`, never a local copy that may be stale.

Also note the repo name as Shipyard knows it (the `repo_name` in the environment payload, usually
the GitHub repo name without the org prefix).

If the user asked you to verify a different branch or repository than the one checked out, use
what they named instead, for `BRANCH` and the repo name alike. For a branch that is not checked
out, `PUSHED_SHA` is `git rev-parse origin/<branch>` after a `git fetch`, not your local `HEAD`,
and everything below (reading the diff and the tests, writing checks, committing) happens in a
separate checkout of that commit (`git worktree add <dir> <PUSHED_SHA>`), never in the one you
started in.

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
| Exactly one environment | Continue to Step 2b |
| No environments | The push may not have registered. Retry per the polling contract; after 2 minutes stop and tell the user |
| More than one | **Stop. Never guess.** List id, url, branch, and commit for each, and ask which one |

## Step 2b — Plan the checks, and get them approved

Before running anything, write down what you will check and get the user's agreement. The agent
that made a change shares its blind spots with any plan it writes for itself, so the plan starts
from fresh eyes and covers the change's blast radius, not only the change.

**Skip this step in a read-only run** (5c): nothing will be added, so there is nothing to approve.
Go to Step 3, and assess and report coverage as usual.

**Drafting.** If you can start a subagent with a fresh context (no copy of this conversation),
give it only: the output of `git diff origin/<BASE>...PUSHED_SHA`, read access to the repository,
the checklist below, and the acceptance check if there is one. Ask it for the plan table, and use
what it returns. If you cannot start a subagent, draft the plan yourself from that diff before
re-reading this conversation. Either way, say which you did at the top of the plan (`drafted by a
fresh subagent` or `drafted from the diff by the agent that made the change`).

**Blast-radius checklist.** Consider every area; list an item only when the diff gives a reason,
and cite the diff lines that give it.

| Area | What to look for | Typical check |
|---|---|---|
| The change itself | Behavior the diff adds or alters | Assert on the new value |
| Callers and consumers | Other routes, jobs or UI that use a changed function, template or API field (search the code) | Exercise one or two callers |
| Shared pieces | Templates, partials, components or config included in more than one place | Check the other pages that include it |
| Access and security | New routes without authentication, changed permission checks, newly public data | Request it without credentials and assert the intended refusal or access |
| Data | Migrations, schema, stored formats | Read existing data after the build |
| Other services | Workers, queues, sibling repositories in the same environment | A log or query check in that service through `exec_service` |
| Existing behavior | Everything else | The acceptance check, as the regression net |

**The plan** is one numbered table, followed by one line on how to answer:

```
Test plan for <PUSHED_SHA> (drafted by a fresh subagent)

| # | Area | Why at risk (diff lines) | Check (tool + assertion) | New/changed | Writes data? | Rough time |
|---|------|--------------------------|--------------------------|-------------|--------------|------------|
| 1 | Acceptance check | ... | ... | ... | ... | ... |

Reply: yes · drop 3 · add: <what> · change 2: <how> · no
```

The acceptance check (5a), when there is one, is always item 1. `New/changed` follows the rule in
5b and decides whether 5d's base check applies. A check that writes data is marked, because it
never runs against the base environment.

**Approval.** Send the plan as message 2 and **stop until the user answers**: do not poll, run
checks or change anything while you wait. Then:

- `yes` → the plan is accepted as shown.
- `drop <n>`, `add: ...`, `change <n>: ...` → revise the plan and send it again, and stop again.
  Items the user dropped stay in the table, marked `dropped by user`.
- Edits that also approve (`drop 3, otherwise yes`, `add: X and go`) → apply them and run the
  revised plan without asking again; show the revision in the report. Ask again only when an
  edit leaves you unsure what they want.
- `no` → do not run anything. Report `Not verified: the test plan was rejected`, with the plan and
  any reason the user gave, and stop.

**Auto-approve.** When the user said "auto-approve" (or "don't wait for me") for this run, or the
repository has a `Verification: auto-approve` line and the user did not ask to approve, do not
wait: accept your own plan and put it in the final report. Unattended runs, such as CI, need this;
without it they stop here with nothing verified.

## Step 3 — Poll until your commit is serving

**Polling contract:**

```
Loop:
  get_environments(branch=BRANCH, repo_name=<repo>)
  find projects[] entry where repo_name == <repo>

  stopped == true or retired == true    → STOP POLLING. The environment is not running;
                                          see "Stopped environments" below
  processing == true                    → a build is in flight, keep polling. Check this
                                          before commit_hash: a new environment's first
                                          build reports a null commit_hash for minutes
  commit_hash is null, not processing   → no running build. Treat as stopped/retired above,
                                          not as a mismatch
  commit_hash == PUSHED_SHA AND ready   → SERVING. Stop polling, go to Step 4
  commit_hash == PUSHED_SHA, ready=false→ keep polling. The commit lands BEFORE the
                                          environment serves it (measured: ~40s apart)
  commit_hash != PUSHED_SHA             → your build has not landed, keep polling
  no environment in response            → keep polling (see Step 2 timeout)

  sleep: 5s, then 10s, 20s, 40s, 60s, 60s...   (cap 60s)
  give up after 20 minutes → report BUILD_TIMEOUT with the last commit_hash and flags seen
```

**Stopped environments.** `stopped` or `retired` true, or a null `commit_hash` while nothing is
processing, means no build is running and none is coming. Polling will never succeed. Stop and tell the user the environment is
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
as the `shipyard_token` cookie. The `?shipyard_token=` query parameter also works, but it lands in
server logs and shell history, so do not use it.

Two things to know:
- This gets you past Shipyard's gate. It does **not** log you into the application. If the app has
  its own sign-in, the acceptance command handles that.
- **Never** print the token, paste it into chat, commit it, put it in a PR comment, or write it to
  a log. Pass it through an environment variable to the acceptance command.

## Step 5 — Check the change, not just the build

### 5a — Run the acceptance check, if there is one

Take the acceptance check from the first of these that has one:

1. The `acceptance` argument, when this prompt was invoked with it. It appears under
   "This invocation" at the end of these instructions. If it looks cut off (a quote that opens
   and never closes, or a sentence that stops mid-way), take it from the full text the user
   typed with the command instead.
2. Whatever the repository documents in `CLAUDE.md`, `AGENTS.md` or a README section. Prefer a
   line labelled `Acceptance check:`. It only counts if it exercises the running environment:
   a unit-test command that never touches `url` is not an acceptance check.
3. Nothing. That is a valid answer; see below.

An acceptance check is either a **command** or a **description** of the expected behavior. Decide
by how it reads, not by whether its first word happens to be a program:

- **A command** reads as shell: a program or script followed by arguments, flags, paths or
  targets, possibly after `VAR=value` assignments (`npm run test:e2e`, `make e2e`,
  `./scripts/check.sh`, `CI=1 pytest -k login`). A path-like first word (`./`, `/`, `scripts/`)
  is a command even if the file is missing, and then it is FAILED as not found.
- **A description** reads as a sentence about behavior ("the header is blue", "make sure signup
  sends an email", "`GET /api/widgets` returns `count` as a number"), even when it starts with a
  word that is also a program (`make`, `test`, `open`, `find`).
- When it could be either, it is a description; say in the report which way you read it.

**With a command:** run it with the environment in two variables, set only for that command:

```
SHIPYARD_URL=<url> SHIPYARD_TOKEN=<bypass_token> <acceptance command>
```

The command decides pass or fail, not you. "The page returned 200" is not verification. A command
that cannot run at all (not found, or it crashes before reaching `url`) is a **FAILED** result with
that error, never a reason to fall back to the Serving report below.

**With a description:** write a check that asserts exactly what it says, with the rules and tools
of 5c (reproducible, asserting on the behavior itself, token kept out of the report), and run it.
The description decides pass or fail: check the stated result, not something easier nearby. Do
this even in a read-only run, because the user asked for this check; in that case run it and
quote it, but do not commit a test for it. A description about a fix or changed behavior also
needs the base check in 5d. If it cannot be captured as a command (for example "the page feels
faster"), what you saw is **Observed**, never Verified. Always quote the description in the report
next to the check it became, so the user can see how you read it.

**Without one:** do not invent a replacement suite, do not stop, and do not ask the user to
configure one mid-run. Go on to 5b: the change can still be checked directly.

### 5b — Is the change itself covered?

A passing suite proves nothing regressed. It does not prove the new behavior works, because a new
feature usually has no test yet. The behavior to cover is the **accepted plan's items** (Step 2b),
not a fresh reading of the diff; in a read-only run, where there is no plan, read
`git diff origin/<BASE>...PUSHED_SHA` and list the behavior it changes.
Mark each item **new** or **changed** by what its check will assert, not by the route or page it
lives on:

- **new** — the asserted thing is absent on `BASE`: a new endpoint, page, command, field, header or
  element, even when it is added to a route that already exists. Quote the diff lines that add it.
- **changed** — the asserted thing exists on `BASE` with a different value or behavior: a bug fix,
  changed text, a changed status code, a changed default.
For each item, name the test in the acceptance check that exercises it **and quote the assertion**
that checks the behavior. A test only counts if this run's output shows it ran and passed against
the environment: one that was skipped, filtered out, or run against mocks does not cover anything.

- **full** — every behavior listed here has a named test with a quoted assertion that ran.
- **partial** — some do.
- **none** — none do, or there is no acceptance check.

A test whose name sounds related but whose assertions do not touch the behavior does not count.
"Coverage looks fine" is not an assessment.

### 5c — Add a check for what is not covered

Skip this step when the run is **read-only**: the user asked for that in the conversation ("don't
add any checks", "read-only", "just verify, don't touch anything"), or the repository has a
`Verification: read-only` line and the user did not ask for checks. What the user says for this
run outranks the repository's line, either way; passing an `acceptance` check is not asking for
more checks, and does not lift read-only. Coverage is still assessed and reported.

Otherwise, check each uncovered plan item, using the check the plan names unless it proves
impossible (then say why in the report). Run nothing that is not in the accepted plan. A check is
not limited to end-to-end tests:

| The change | A check that exercises it |
|---|---|
| UI | An e2e test in the repository's own framework |
| API or backend | An API test in the repository's test framework, or `curl` requests against `SHIPYARD_URL` with the expected status and body |
| No HTTP surface (worker, migration, cron) | A CLI call, or a query or log read inside the service through `exec_service`, with the expected output. Trigger the behavior during the run and read only what that trigger produced (filter by a value you created or a timestamp after it); a row or log line that already existed proves nothing about this commit |
| A route `url` does not expose (404 at the ingress, internal or health paths) | The same request from inside the service through `exec_service`, for example `curl -s localhost:<port>/<path>`, with the expected output |

Rules:

- **Assert on the behavior itself.** Check the new field's value, the fixed message, the changed
  status: something only the change produces. "Returns 200" or "the page loads" is not a check of
  the change.
- **Reproducible or it does not count.** Prefer a test committed in the repository's framework.
  Otherwise put every command in the report verbatim, with the expected and actual output, so a
  reviewer can rerun it. A script you wrote for the check is part of the command: commit it, or
  quote its full contents in the report; its name and assertion alone cannot be rerun. A check that exists only in your reasoning ("I looked, it worked") is
  **Observed**, never part of Verified.
- **Keep the token out of everything you report.** Send it only as a cookie from the variable,
  for example `curl -b "shipyard_token=$SHIPYARD_TOKEN" "$SHIPYARD_URL/..."`, never on the URL
  and never with `-v`. Before quoting any command or output, remove the token's value and any
  application credentials.
- **Cover only what is uncovered.** Never edit or weaken an existing test to make it pass.
- **A committed test is part of the change.** Commit it, push, set `PUSHED_SHA` to the new
  `HEAD`, and go back to Step 3. The environment has to serve the commit that contains the test,
  and the test has to be run again there: a result from before the push does not count.

### 5d — Prove checks for changed behavior are real

A check for a **new** behavior cannot pass on `BASE`, because the behavior is not there; its
quoted assertion and the quoted diff lines that add it are the proof. If a usable base environment
exists anyway (below), run it there too: a "new" check that passes on base was mislabeled. A check for a **changed** behavior can: a check that passes both
before and after the change is not testing it. Run each new check for a changed behavior against
two environments:

- **This environment** — it must pass.
- **The base environment** — it must fail, at the assertion on the changed behavior.

Find the base environment with `get_environments(branch=BASE)` and the same `repo_name` filter.
Use it only if all of these hold, otherwise report `base: not checked` with the reason:

- Exactly one of the environments that come back is `ready` and not `stopped` or `retired`. A
  base branch can have several (a detached one alongside the regular one); ignore the ones that
  are not running. Two or more ready ones is ambiguous: report `base: not checked`. The list can
  report `ready: false` for an environment that is serving: when one is at the branch point and
  not stopped, confirm with `get_environment(<id>)` and use its `ready` instead.
- Its `commit_hash` for this repo is exactly the commit your branch started from:
  `$(git merge-base origin/BASE PUSHED_SHA)`. An older base can fail the check because of some
  other bug fixed since, which would prove nothing; a newer one may already contain parts of the
  change. Otherwise report `base: not checked (base is not at the branch point)`, and tell the user
  that merging `BASE` into the branch moves the branch point to what the base environment serves.
- The check does not change state. The base environment belongs to the team: **never restart it,
  never edit it, and never run anything against it that writes** (no POSTs, signups, form posts,
  or exec writes). A check that writes is reported `base: not checked (writes data)`.

For HTTP checks, point `SHIPYARD_URL` at the base environment and use its own `bypass_token`.

| Result | Meaning |
|---|---|
| Fails on base at the assertion, passes here | Proven. Quote the failing assertion and record the base commit |
| Fails on base before the assertion (404 on setup, auth, missing data, connection) | Not proven: the failure says nothing about the change. Report `base: not checked` |
| Passes on both | It does not test the change. Rewrite it once; if it still passes on both, report it as not proven |
| Fails here | A real failure: go to Failure handling. Do not edit the check to make it pass, unless it is provably wrong about the intended behavior, and then say why in the report |

## Step 6 — Re-check the commit before you report

Call `get_environments` once more and compare `commit_hash` against `PUSHED_SHA`.

- Unchanged → your result is valid, go to Step 7.
- Changed because **you** pushed (a test in 5c, or a fix) → not a conflict. Set `PUSHED_SHA` to
  your new commit and return to Step 3.
- Changed by anyone else → their build landed mid-run and your result describes neither commit.
  **Discard it and return to Step 3.** Do this at most twice, then stop and tell the user the
  environment is too busy to verify against right now.

If you edited the running container at any point, the final result must come from after a rebuild
of your pushed commit: rerun the mapping probe (below) on every file you edited and confirm each
now matches `PUSHED_SHA`. A file that still differs means the container keeps source across
rebuilds; report that instead of a result.

## Step 7 — Report

Every form except a rejected plan carries a `Plan:` block, one line per plan item, and a
`Coverage:` line:

```
  Plan:        (drafted by a fresh subagent; approved by the user, or auto-approved)
    1. <area>: <check>        passed, base: failed ✓ @ <sha>
    2. <area>: <check>        failed: <first real error>
    3. <area>: <check>        skipped: <reason>
    4. <area>: <check>        dropped by user
```

Every form carries a `Coverage:` line. Mark each check you added `(agent-written, review it)` and
give its base result: `base: failed ✓ @ <base sha>` (changed behavior, proven), `base: not needed
(new behavior)`, or `base: not checked (<reason>)`. List checks that are not committed tests
(`curl`, CLI, `exec_service`) under `Checks run:`, verbatim, token removed, with expected and
actual output.

**Verified** — every accepted plan item passed, the acceptance check passed (if there is one),
coverage is full, every check you added is reproducible, and every one for a behavior marked
changed failed on base:

```
Verified on Shipyard.
  Environment: <url>
  Commit:      <PUSHED_SHA>  (confirmed serving before and after the run)
  Check:       <acceptance command, or "<description>" → the check it became, or none configured>
  Result:      PASS
  Coverage:    full — <named tests and checks>
  Checks run:  <verbatim commands, expected and actual output>
```

**Passed, not covered** — everything that ran passed, but some behavior listed in 5b has no proven
check (the run was read-only, a new check could not be proven on base, or none could be written):

```
Passed on Shipyard, but the change is not fully covered, so this is not verified.
  Environment: <url>
  Commit:      <PUSHED_SHA>
  Check:       <acceptance command, or "<description>" → the check it became, or none configured>
  Result:      PASS
  Coverage:    partial — <what is covered>; not covered: <what is not, and why>
```

**FAILED**:

```
Verification FAILED on Shipyard.
  Environment: <url>
  Commit:      <PUSHED_SHA>
  Check:       <acceptance command or the failing check>
  Failing:     <test names, or the first real error>
  Logs:        <the relevant lines, not the whole dump>
```

**Observed** — the only evidence is something you looked at and could not capture as a
reproducible command, such as a UI with no e2e framework:

```
Observed on Shipyard. No reproducible check covers this, so it is not verified.
  Environment: <url>
  Commit:      <PUSHED_SHA>
  Check:       none configured, or "<description>" that could not be captured as a command
  Observed:    <what you looked at and what you saw>
```

**Serving** — nothing ran: no acceptance check, and no check was added:

```
Serving your commit on Shipyard. No acceptance check ran, so this is not verified.
  Environment: <url>
  Commit:      <PUSHED_SHA>  (confirmed ready and serving)
  Check:       none configured
  Coverage:    none
```

After Serving or Passed, not covered, add one line, once, so the user knows the options: a check
can be passed as `acceptance` when invoking this prompt, as a command or a plain description of
the expected behavior, or documented in `CLAUDE.md` or
`AGENTS.md`.

Omit any line you do not have, except `Check:` and `Coverage:` in every form and `Result:` on
success: a success report without them is not verification. Never report PASS for a run you had
to discard, and never report Verified from a container you edited (see below).

## Failure handling

**In a read-only run, do not fix anything:** no edits, commits or pushes. Report the failure with
what you found in the logs, and stop.

**Three attempts, then stop.** After a failed check: read the failure and the environment logs,
make one targeted fix, and try again. An attempt is one fix, whether you pushed it or tried it in
the running container. **Stop after 3 failed attempts** and report what you tried, what failed
each time, and the environment URL. Looping past that burns build capacity and rarely converges.

The accepted plan carries over to each attempt: run it again on the new commit. If a fix touches
areas the plan does not cover, add items for them and show just those for approval (Step 2b),
unless the run is auto-approved.

Each attempt normally means push, return to Step 3 with the new SHA, and wait for a rebuild. When
the container can be edited in place (next section), try the fix there first and push once it
passes. That makes attempts cheaper; it does not raise the limit.

## Iterating in the running container

A rebuild per attempt takes minutes. If the service runs a dev server that reloads code, you can
write changed files straight into the running container, rerun the checks in seconds, and push
once everything passes.

**Only when all of these hold:**

- `exec_service` is enabled. If its description says DISABLED, skip this section.
- The environment is this change's own. **Never** edit the base environment. Others reviewing this
  change may be using its environment; edits there are visible to them until the rebuild.
- The run is not read-only (5c).
- Both probes below pass. They fail closed: any doubt means push per attempt instead.

`exec_service` takes the environment, a `service_name` (list them with `get_services`) and the
command as an argument array. It has no stdin, stops waiting after 60 seconds, and cuts output at
64KB. Pipes and redirects need a shell: `["sh", "-c", "..."]`. If the image has no `sh`,
`base64` or `sha256sum`, stop here.

**Mapping probe.** Find where the repository lives in the container, then look for two or three
files the diff touches. Compare each file's `sha256sum` in the container with
`git show <PUSHED_SHA>:<path> | sha256sum` locally. Every one must match. A mismatch or a missing
file means the container does not serve these files from source; stop here.

**Reload probe.** Look at every process, not only process 1, which is often `tini`, `sh -c` or
`npm`, but **print only the watcher names you match, never whole command lines**: those can carry
passwords and tokens. For example:
`sh -c 'cat /proc/[0-9]*/cmdline 2>/dev/null | tr "\0" " " | grep -oE "nodemon|vite|next dev|webpack serve|flask --debug|uvicorn .*--reload|air|rails server" | sort -u'`.
One of them must be a known watcher (`nodemon`, `vite`, `next dev`, `webpack serve`,
`flask --debug`, `uvicorn --reload`, `air`, `rails server` in development). If none is, stop
here; do not stretch the list to fit.

**Writing a file.** Base64-encode it locally and split the encoded text into chunks of at most
32KB, each a multiple of 4 characters. Append each chunk to a temporary file next to the target,
decode it, compare its `sha256sum` with the local file, then `mv` it over the target, so the
watcher never reloads a half-written file. A mismatch means the write failed: remove the temporary
file and push per attempt instead.

**Rules:**

- Results from an edited container are **provisional**. The code there matches no commit, and
  Shipyard still reports the old `commit_hash`. Never report Verified from it.
- When the checks pass, commit the fix and any new tests, push once, and continue from Step 3 on
  the new commit. Step 6 confirms the rebuild replaced every file you edited.
- If an edit does not change the behavior although the probes passed, the reload is not working.
  Stop editing, push per attempt, and say so.
- If you stop without pushing, or you pushed but the rebuild failed or timed out so the edited
  container is still the one serving, restore every file you edited to its `PUSHED_SHA` content
  with the same write and check, delete every file you created, and say the container was
  restored. Before restoring, check each file's hash still matches what you wrote: if it does not,
  something else replaced the container, so leave it alone.

## Troubleshooting

| Symptom | Likely cause | Action |
|---|---|---|
| Empty response for a repo you know has environments | Wrong org, or an expired token | Re-run the Step 0 preflight; `get_orgs` tells you which |
| `commit_hash` never matches | You pushed to a different remote or branch than the environment tracks | Confirm the branch and remote, then re-check |
| Matching commit but the app 404s or redirects to a login | Shipyard's gate is passed; this is the app's own auth or routing | The acceptance command must handle app login |
| Multi-repo environment, wrong code tested | Matched the wrong `projects[]` entry | Always filter by `repo_name` before reading `commit_hash` |
| Build failed instead of completing | Real build failure | Read build logs, fix the cause, push; do not rebuild unchanged code |
| Polling never finishes, flags look inert | `stopped` or `retired` is true, or `commit_hash` is null and nothing is processing | The environment is not running. Tell the user; do not restart it yourself |
| 302 to `/oauth2/sign_in` | The bypass token was not sent, or was sent on the wrong host | Send it as the `shipyard_token` cookie on the environment's own host |

## What counts as done

**Verified** means all of these, on the commit you finally pushed: the environment served **your**
commit and it had not changed when the run finished; every item of the approved (or auto-approved)
test plan passed; the acceptance check, if there is one, passed; every behavior the change touches is covered by a named test or check with a quoted
assertion; every check you added is reproducible; and every added check for a changed behavior
failed on the base environment at that assertion. Nothing from an edited container counts.

**Passed, not covered** means everything that ran passed, but some behavior listed in 5b has no
proven check. Say what is missing.

**Observed** means the only evidence is something you looked at and could not capture as a
command.

**Serving** means the build landed and the environment is up, and nothing ran.

**FAILED** means a check failed on the pushed commit.

**Not verified: plan rejected** means the user said no to the test plan, and nothing ran.

Report the one that is true, in those words. Each is a useful, honest answer; reporting anything
less than Verified as verified is worse than reporting nothing.
