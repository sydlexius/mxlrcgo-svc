---
description: "Check all PRs in a stack for pending bot reviews, then triage/fix/reply in dependency order"
argument-hint: "[PR numbers (e.g. 32-36 or 32,33,35) -- defaults to auto-detect from current branch]"
tools:
  bash: true
  glob: true
  grep: true
  read: true
  edit: true
  write: true
  task: true
---

# Review Stack

Find all PRs in a stack that have pending bot reviews (CodeRabbit, Copilot), then process
them in dependency order using `/handle-review` discipline. Fixes cascade: after fixing a
base PR, restack before handling the next PR up the chain.

**Arguments (optional):** "$ARGUMENTS"

**Special arguments:**
- `--clear`: Clear the session breadcrumb file and exit. Use at the start of a new
  working session to reset state from previous sessions.
- `--list`: Show the contents of the session breadcrumb file (unique, still-open PRs)
  without processing any reviews. Useful to see what would be reviewed.

```bash
SESSION_PRS_FILE="/tmp/mxlrcgo-svc-session-prs.txt"

if [ "$ARGUMENTS" = "--clear" ]; then
  rm -f "$SESSION_PRS_FILE"
  echo "Session PR breadcrumbs cleared."
  exit
fi

if [ "$ARGUMENTS" = "--list" ]; then
  if [ -f "$SESSION_PRS_FILE" ]; then
    echo "Session PRs:"
    sort -un "$SESSION_PRS_FILE"
  else
    echo "No session PRs recorded."
  fi
  exit
fi
```

**Parallelism strategy:** Data gathering (trigger, poll, fetch, pre-triage) is parallelized
across PRs using foreground agents. Fixing is serial because base PR fixes cascade into
dependent PRs via rebase.

---

## Step 1 -- Identify the stack

This step runs first because all subsequent parallel work depends on knowing the PR list.

Resolve `repo` and `me`:

```bash
repo=$(gh repo view --json nameWithOwner --jq .nameWithOwner)
me=$(gh api user --jq .login)
```

### Option A: Explicit PR numbers

If `$ARGUMENTS` contains numbers (e.g. `32-36`, `32,33,35`, or `32 33 35`):
- Parse into a list of PR numbers
- `32-36` means PRs 32, 33, 34, 35, 36
- `32,33,35` or `32 33 35` means those specific PRs

### Option B: Graphite auto-detect

If `$ARGUMENTS` is empty and `gt` is available:

```bash
command -v gt >/dev/null 2>&1 && gt log short 2>/dev/null
```

If Graphite outputs a stack, extract PR numbers from its output.

### Option C: Session breadcrumb file (default)

If `$ARGUMENTS` is empty and Graphite is unavailable, read PR numbers from the
session breadcrumb file. Other commands (`/prep-pr`, `/handle-review`) append PR
numbers to this file as they push PRs, so it tracks exactly what was worked on.

```bash
SESSION_PRS_FILE="/tmp/mxlrcgo-svc-session-prs.txt"

if [ -f "$SESSION_PRS_FILE" ]; then
  # Read unique PR numbers, filter to still-open PRs
  session_numbers=$(sort -u "$SESSION_PRS_FILE")
  open_prs=$(gh pr list --state open --json number,baseRefName,headRefName)
  # Intersect: keep only PRs that are both in the file and still open
fi
```

Filter `open_prs` to those whose `number` appears in `session_numbers`.
Sort in dependency order (base PRs first, using the base->head chain).

If the breadcrumb file is missing or empty, or the intersection yields nothing,
fall back to the current branch's PR and its chain:

```bash
current_pr=$(gh pr view --json number,baseRefName,headRefName \
  --jq '{number, base: .baseRefName, head: .headRefName}')
```

Walk UP: check if the base branch also has an open PR, repeat until `main`/`master`.
Walk DOWN: check if any other open PR uses the current branch as its base.

If still no PRs found, stop: "No session PRs detected. Pass PR numbers explicitly: `/review-stack 32-36`"

Save the original branch name so you can return to it at the end.

---

## Step 2 -- Parallel: trigger reviews, poll readiness, fetch comments

After identifying the stack, launch **one foreground Agent per PR** in a single message
(all agents in parallel). Each agent performs the full data-gathering pipeline for its PR.

### Agent prompt template (one per PR)

Each agent receives this task:

> You are gathering bot review data for PR #<number> in repo <repo>.
>
> **Step A -- Trigger CodeRabbit review if needed (with rate-limit awareness):**
> ```bash
> base=$(gh pr view <number> --json baseRefName --jq .baseRefName)
> default=$(gh api "repos/<repo>" --jq .default_branch)
> if [ "$base" != "$default" ]; then
>   cr_count=$(gh api "repos/<repo>/pulls/<number>/reviews" --paginate \
>     --jq '[.[] | select(.user.login == "coderabbitai[bot]")] | length')
>   if [ "$cr_count" -eq 0 ]; then
>     bash $HOME/.claude/scripts/pr-unreplied-comments.sh --trigger-cr <number> <repo>
>   fi
> fi
> ```
> The `--trigger-cr` flag checks for a dangling rate limit message, waits the
> remaining time if needed, then posts `@coderabbitai review`.
>
> **Step B -- Poll for review readiness (geometric cooldown):**
> Poll at 15s, 30s, 60s, 120s intervals. At each interval check:
> ```bash
> pending=$(bash ~/.claude/scripts/pr-unreplied-comments.sh --pending-only <number>)
> unreplied=$(bash ~/.claude/scripts/pr-unreplied-comments.sh --count-only <number>)
> ```
> Ready when `pending == 0` AND `unreplied` count matches the previous check.
> If not stable after 4 polls, report the PR as WAITING with details.
>
> **Step C -- Fetch all unreplied bot comments (overview then full bodies):**
> ```bash
> bash ~/.claude/scripts/pr-unreplied-comments.sh <number>
> bash ~/.claude/scripts/pr-read-comments.sh <number>
> bash ~/.claude/scripts/pr-read-comments.sh --reviews <number>
> ```
> For specific comment IDs only:
> ```bash
> bash ~/.claude/scripts/pr-read-comments.sh <number> <id1> <id2> ...
> ```
>
> **Return format:**
> ```
> PR: #<number>
> Branch: <head_branch>
> Status: NEEDS WORK / WAITING / CLEAN / NO REVIEWS
> Unreplied comments: <count>
> CHANGES_REQUESTED reviews: <count>
> Comments:
> - id: <id>, user: <login>, path: <path>, line: <line>, commit: <sha>, stale: <true/false>, body: <full body>
> - ...
> ```

**Bot identity notes (important -- include in each agent prompt):**
- CodeRabbit reviews: `coderabbitai[bot]`
- Copilot reviews: `copilot-pull-request-reviewer[bot]`
- Copilot inline comments: `Copilot` (different login, same user ID 175728472)
- Always check BOTH Copilot logins when scanning for comments

**Skip the cooldown** if the user explicitly says reviews are done or asks to proceed
immediately. In that case, skip Step B in the agent prompt and go straight to Step C.

### After all agents return

Collect results from all agents into a unified status table:

```
| PR   | Branch              | Status     | Unreplied | CR blocked? |
|------|---------------------|------------|-----------|-------------|
| #32  | feat/32-foo         | NEEDS WORK | 3         | Yes         |
| #33  | feat/33-bar         | CLEAN      | 0         | No          |
| #34  | feat/34-baz         | WAITING    | ?         | --          |
```

If any PRs are WAITING, report which bots are still pending and ask:
"N PRs still waiting for reviews. Proceed with available reviews, or keep waiting?"

If no PRs need work, say: "All PRs in the stack are clean. Nothing to do." and stop.

---

## Step 3 -- Parallel: pre-triage analysis

For each PR with status `NEEDS WORK`, launch **one foreground Agent per PR** in a single
message to pre-compute the triage. These agents run in parallel.

### Agent prompt template (one per PR)

Each agent receives:

> You are pre-triaging bot review comments for PR #<number> in repo <repo>.
> The PR is on branch <branch>. Here are the unreplied bot comments:
>
> <paste all comments from Step 2 agent output for this PR>
>
> The full PR stack is: <list all PR numbers in dependency order>
> This PR is at position <N> in the stack.
>
> For each comment, read the relevant source code and assign a category:
>
> | Category | Description | Default action |
> |----------|-------------|----------------|
> | **bug** | Real code defect | Fix now |
> | **test-gap** | Missing test, untested edge case | Fix now |
> | **rename-incomplete** | Old name still referenced somewhere | Fix now (batch) |
> | **style** | Formatting, naming, comment wording | Fix now |
> | **false-positive** | Incorrect or inapplicable suggestion | Rebut |
> | **stacked-pr-repeat** | Flags "dead/unused" code wired in a later PR in the stack | Dismiss |
> | **architectural** | Requires design change beyond this PR's scope | Defer (must justify) |
>
> **Known false-positive patterns (auto-classify without investigation):**
> - Copilot "PR title scope" complaints about issues added in follow-up commits
> - Copilot flagging exported-but-not-yet-called methods as unused
>
> **Stacked PR repeats:** Comments flagging code as "dead" or "unused" when it is
> wired in by a later PR (#<later_pr_numbers>) should be categorized as
> `stacked-pr-repeat`. Note which PR wires it in.
>
> **Stale-diff check:** If ALL comments have `stale: true`, note that this PR is a
> candidate for the stale-diff fast path (batch reply "already fixed in HEAD").
>
> **Grouping:** If multiple comments share the same root cause (e.g., "rename X" at
> lines 50, 120, and 200), group them as a single fix item.
>
> **Return format:**
> ```
> PR: #<number>
> Stale-diff fast path: yes/no
>
> | # | Category | File:Line | Commit | Stale? | Summary | Action | Group |
> |---|----------|-----------|--------|--------|---------|--------|-------|
> | 1 | bug | internal/app/app.go:45 | abc1234 | no | context not propagated | Fix now | -- |
> | 2 | false-positive | internal/musixmatch/client.go:90 | def5678 | yes | safe file path | Rebut | -- |
>
> Fix items: <count>
> Rebut items: <count>
> Dismiss items: <count>
> Defer items: <count>
> ```
>
> Do NOT make any code changes. This is analysis only.

### After all triage agents return

Merge the triage results into a combined view. Present to the user:

```
## Pre-triage results

### PR #32 (3 comments)
| # | Category | File:Line | Summary | Action |
|---|----------|-----------|---------|--------|
| 1 | ... | ... | ... | Fix now |
...

### PR #33 (1 comment)
...

Process N PRs that need work? (yes / pick specific PRs / skip)
```

Ask the user to confirm or adjust using selectable choices:
- "Looks good, proceed with fixes"
- "Adjust category for comment N on PR #X"
- "Show me more context for comment N on PR #X"

Wait for confirmation before proceeding to fixes.

---

## Step 4 -- Serial: fix PRs in dependency order

For each PR that `NEEDS WORK`, in dependency order (base PR first):

### 4a. Stale-diff fast path

If the pre-triage flagged this PR as stale-diff-fast-path eligible (all comments are
stale), offer the fast path:

"All N comments on PR #X target commit XXXX (HEAD is YYYY). Batch-reply all as
'already fixed in HEAD'?"

If confirmed, skip to Step 4f (reply) with "Already addressed in HEAD." for each comment.

### 4b. Switch to the PR's branch

```bash
git checkout <branch_name>
git pull --rebase origin <branch_name>
```

### 4c. Execute fixes

Using the pre-computed triage (no need to re-analyze), for each "Fix now" item:
1. Read the relevant file and understand the surrounding context
2. Make the fix
3. Track what was changed for the reply

For each "Fix now (batch)" group:
1. Find ALL instances across the codebase (not just the lines flagged)
2. Fix all instances in one pass

For each "Defer" item:
1. Create a tracking issue using the `new-issue` command
2. Note the issue number for the reply

**Do not commit between fixes.** Make all changes first, then commit once.

### 4d. Run verification

```bash
go test -count=1 -race ./... 2>&1
golangci-lint run ./... 2>&1
```

If tests or lint fail, fix the failures before proceeding.

### 4e. Commit

```bash
git add <specific files that were changed>
git commit -m "fix: address PR review feedback

- <one-line summary per fix>"
```

### 4f. Push

Push before posting any replies -- this ensures the SHA referenced in "Fixed in <sha>"
replies is reachable on the remote. If push fails, **stop here** and do not post replies.

```bash
git push origin <branch_name>
```

After a successful push, record the PR number in the session breadcrumb file:

```bash
SESSION_PRS_FILE="/tmp/mxlrcgo-svc-session-prs.txt"
echo "<PR>" >> "$SESSION_PRS_FILE"
```

### 4g. Reply to all comments in batch

Only post replies after a successful push.

**Fix now:**
```bash
bash ~/.claude/scripts/reply-comment.sh <PR> <comment_id> 'Fixed in <short-sha>.'
```

**Rebut:**
```bash
bash ~/.claude/scripts/reply-comment.sh <PR> <comment_id> '<evidence-based explanation of why this is not an issue>'
```

**Stacked-PR repeat:**
```bash
bash ~/.claude/scripts/reply-comment.sh <PR> <comment_id> 'This is wired in PR #<later_pr>. Per-PR review limitation on stacked PRs.'
```

**Defer:**
```bash
bash ~/.claude/scripts/reply-comment.sh <PR> <comment_id> 'Tracked in #<issue-number>. Requires <brief justification for deferral>.'
```

After all inline replies, resolve threads:

```bash
# Copilot threads
bash ~/.claude/scripts/resolve-threads.sh <PR> <copilot_comment_id> ...

# CodeRabbit threads (one PR-level comment resolves all)
bash ~/.claude/scripts/reply-comment.sh <PR> '@coderabbitai resolve'
```

### 4h. Cascade check

After pushing fixes to a base PR, ask:

```
Fixes pushed to #32. This may affect PRs higher in the stack.
Restack now? (yes / no / check first)
```

If yes:

```bash
# Without Graphite (manual rebase chain):
git checkout <next_branch>
git rebase <fixed_branch>
# ... repeat up the chain
```

If rebase conflicts occur, stop and report them. Do not force-resolve conflicts.

**Important:** If a restack changes code that was pre-triaged in Step 3, the triage
for affected PRs may be stale. After restacking, re-check whether pre-triaged comments
still apply to the rebased code. If a comment's target file/line shifted significantly,
note this during the fix phase and adjust.

### 4i. Move to next PR

After restacking (or skipping it), continue to the next PR that needs work.

---

## Step 5 -- Post-fix: assess Copilot re-review

CodeRabbit will automatically review the pushed fixes. Copilot will NOT re-review
automatically (disabled due to 61% repeat noise rate).

Assess whether a manual Copilot re-review is warranted for each fixed PR based on
the scope of changes:

**Recommend re-review** when fixes touched:
- Concurrency primitives or shared state
- Error handling paths (new error branches, changed return values)
- Security-sensitive code (token handling, file paths)
- Substantial new code (not just one-line fixes)

**Skip re-review** when fixes were:
- Comment/doc-only changes
- Trivial one-line corrections
- Test-only changes
- Style/formatting fixes

The GitHub API does not support re-requesting review from bot accounts (422 error).
The user must trigger Copilot re-review manually from the GitHub PR page.

---

## Step 6 -- Summary

```
## Stack review complete

| PR   | Fixed | Dismissed | Rebut | Deferred | Pushed | Copilot re-review? |
|------|-------|-----------|-------|----------|--------|--------------------|
| #32  | 2     | 1         | 0     | 0        | abc123 | Recommended        |
| #33  | 1     | 0         | 0     | 0        | def456 | Not needed         |

Parallelism: Steps 2-3 ran N agents concurrently (data gathering + pre-triage).
Step 4 ran serially in dependency order (cascade constraint).

CodeRabbit will re-review pushed PRs automatically.
Copilot re-review: trigger manually from GitHub for PRs marked "Recommended" above.
```

---

## Important rules

- **Dependency order is mandatory for fixes.** Always fix base PRs before their dependents.
- **Data gathering and triage are parallelized.** Launch all agents in a single message.
- **One commit per PR.** All fixes for a single PR go in one commit.
- **Commit first, reply with SHA, then push.** Same as `/handle-review`.
- **Never force-push** unless the user explicitly asks.
- **Never skip the triage confirmation** for genuinely new comments.
- **Auto-dismiss obvious cross-stack repeats** without asking (e.g. "dead code" that's wired in a later PR).
- **If a restack fails with conflicts,** stop and report. Don't auto-resolve.
- **If a restack invalidates pre-triage,** note it during the fix phase and adjust.
- **CodeRabbit re-reviews automatically; Copilot does not.** Recommend manual Copilot re-review only when fixes are substantial.
- **Save the original branch** so you can return to it at the end.
