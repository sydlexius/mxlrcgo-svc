---
description: "Triage open Copilot/bot PR review comments, fix everything in one pass, reply in batch, push once"
argument-hint: "[PR number -- defaults to current branch's PR]"
tools:
  bash: true
  glob: true
  grep: true
  read: true
  edit: true
  write: true
  task: true
---

# Handle PR Review

Resolve all open Copilot/bot review comments in a single pass. The invariant: **one push,
after all fixes are complete**. Never push per-comment.

This command targets bot reviewer comments (logins containing `copilot` or ending with
`[bot]`). For human reviewer comments, apply the same triage and fix discipline manually.

**PR number (optional):** "$ARGUMENTS"

---

## Step 1 -- Identify the PR

Resolve `pr_number` and `repo`:

```bash
repo=$(gh repo view --json nameWithOwner --jq .nameWithOwner)
me=$(gh api user --jq .login)
```

If `$ARGUMENTS` is a number, use it directly:

```bash
pr_number="$ARGUMENTS"
```

Otherwise detect from the current branch:

```bash
pr_number=$(gh pr view --json number --jq .number)
```

Print the PR URL for confirmation:

```bash
gh pr view "$pr_number" --json url --jq .url
```

If no PR found, stop: "No open PR found for this branch."

---

## Step 1.25 -- Ensure CodeRabbit review exists

Copilot-authored PRs start as drafts and get converted to ready. CodeRabbit does not
automatically review PRs after the draft-to-ready transition, so its review may be
missing entirely.

Check whether CodeRabbit has submitted any review on this PR:

```bash
cr_reviews=$(gh api "repos/$repo/pulls/$pr_number/reviews" --jq '[.[] | select(.user.login == "coderabbitai[bot]")] | length')
```

If `cr_reviews == 0`, trigger a full review:

```bash
gh pr comment "$pr_number" --body "@coderabbitai review"
```

Print: "CodeRabbit review was missing -- triggered via @coderabbitai review."

This must happen before the wait loop in Step 1.5 so the cooldown captures CodeRabbit's
incoming comments.

---

## Step 1.5 -- Wait for bot reviews to complete

Bot reviewers (especially Copilot) post inline comments in waves -- the review status
may show "complete" before all comments have landed. Starting triage too early means
re-triaging when late comments arrive.

**Readiness check:** A PR is ready for triage when BOTH conditions hold across two
consecutive polls:
1. No pending review requests for bot users (Copilot, CodeRabbit)
2. Unreplied bot comment count is stable (same count on two consecutive checks)

**Geometric cooldown:** After triggering reviews (or if reviews were already triggered),
poll with increasing intervals: **15s → 30s → 60s → 120s**. At each interval:

```bash
pending=$(bash ~/.claude/scripts/pr-unreplied-comments.sh --pending-only "$pr_number")
unreplied=$(bash ~/.claude/scripts/pr-unreplied-comments.sh --count-only "$pr_number")
```

If `pending == 0` AND `unreplied` count matches the previous check → ready.
If not stable after 4 polls (~3.75 minutes), tell the user which bots are still
pending and ask whether to proceed or keep waiting.

**Skip the cooldown** if the user explicitly says reviews are done or asks to proceed
immediately.

---

## Step 2 -- Fetch all review comments

```bash
# Unreplied inline comments (truncated first line -- good for triage overview)
bash ~/.claude/scripts/pr-unreplied-comments.sh "$pr_number"

# Full bodies of all unreplied inline comments
bash ~/.claude/scripts/pr-read-comments.sh "$pr_number"

# Full bodies of review-body comments (actionable findings in review summaries)
bash ~/.claude/scripts/pr-read-comments.sh --reviews "$pr_number"

# Full bodies of specific comment IDs only
bash ~/.claude/scripts/pr-read-comments.sh "$pr_number" 123456 789012
```

---

## Step 3 -- Identify open (unreplied) comments

A comment is **open** if:
1. It is a top-level comment (`reply_to` is null) from a reviewer bot (login contains
   `copilot` or ends with `[bot]`, case-insensitive)
2. AND there is no subsequent comment in the same thread from `$me` (the current user)

Build the open list:
- Collect all comment IDs that have a reply from `$me` (where `reply_to` matches
  the reviewer comment's `id`)
- Subtract from the full list of top-level bot/reviewer comments

Print a numbered list of open comments:
```
Open review comments (N total):
1. [id: 123456] path/to/file.go -- "First line of comment body..."
2. [id: 789012] internal/app/app.go -- "First line..."
...
```

If there are no open comments, say: "No open review comments. Nothing to do." and stop.

---

## Step 4 -- Read and categorize each comment

For each open comment:
1. Read the full comment body
2. Read the referenced file and line range in the current codebase
3. Assign one of these categories:

| Category | Meaning |
|----------|---------|
| `bug` | Real code defect -- must fix |
| `test-gap` | Missing test coverage for a real gap -- should fix |
| `false-positive` | Established pattern, known behavior, or intentional design |
| `already-fixed` | Was corrected in a later commit; reply needed but no code change |
| `wont-fix` | Valid suggestion but out of scope for this PR |

### Propagation sweep

Before printing the triage table, check whether any comment is a symptom of a
broader pattern that could recur elsewhere in the codebase. This prevents the
whack-a-mole cycle of fixing one instance per Copilot round.

For each comment about:
- A stale variable or parameter name
- A renamed or replaced function still referenced by the old name
- A stale comment or log message referencing the old behavior
- Any other pattern that is likely copy-pasted across multiple call sites

Run a targeted search before writing any fixes:

```bash
# Example: find all occurrences of the stale name
grep -rn "old_pattern" . --include="*.go"
```

Add every additional occurrence found to the fix scope for that comment. Note them
in the triage table's Summary column so the scope is visible.

Print the full triage table before making any changes:

```
## Triage

| # | ID     | Category       | File              | Summary |
|---|--------|----------------|-------------------|---------|
| 1 | 123456 | bug            | internal/app/app.go | context not propagated |
| 2 | 789012 | false-positive | cmd/mxlrcgo-svc/main.go | godotenv pattern |
| 3 | 345678 | already-fixed  | internal/scanner/scanner.go | fixed in abc1234 |
...
```

Ask: "Does this triage look right? (yes / adjust N to <category>)"

Wait for confirmation before proceeding to fixes.

---

## Step 5 -- Implement all fixes

For every comment categorized as `bug` or `test-gap`:

- Read the relevant code
- Make the minimal correct fix
- Do NOT push yet
- Note the fix briefly for use in the reply

Apply fixes for all comments before moving to the next step.

After all edits are complete, run tests:

```bash
go test -count=1 -race ./... 2>&1
```

If tests fail: stop. Fix the test failures before continuing. Do not reply to comments
or push with broken tests.

Also run the linter:

```bash
golangci-lint run ./... 2>&1
```

If lint errors: fix them before continuing.

---

## Step 5.5 -- Local code review

After all fixes pass tests but before committing, run a focused review of the changed
files to catch any new problems introduced by the fixes:

```bash
git diff --name-only   # identify changed files
```

Launch the `gsd-code-reviewer` agent against the changed files. If it flags **critical**
issues: fix them before committing. Do not push code that will generate a new Copilot
complaint on the next round.

If it flags **important** (non-blocking) issues: present them briefly and ask whether to
fix them now or proceed. Do not block on suggestions.

---

## Step 6 -- Compose replies

For each open comment, draft a reply:

**bug / test-gap (fixed):**
```
Fixed in <sha>. <one-sentence description of what changed>.
```

Get the sha after the fixes are committed (step 7 happens before replies are posted --
see below).

**false-positive:**
```
<Explanation of why this is correct. Reference the specific pattern or architectural
decision if applicable. Keep it brief -- one or two sentences.>
```

**already-fixed:**
```
Fixed in <earlier-sha>.
```

**wont-fix:**
```
Acknowledged. This is out of scope for this PR -- tracking separately as #<issue> or
leaving for a follow-up.
```

---

## Step 7 -- Commit and push

Commit all fixes in a single commit:

```bash
git add -p  # or git add <specific files>
git commit -m "fix: address PR review findings

<bullet list of what was fixed>"
```

Get the short SHA:
```bash
git rev-parse --short HEAD
```

Now substitute the real SHA into all "Fixed in <sha>" reply drafts from step 6.

Push immediately so the SHA is reachable before any replies are posted:

```bash
git push origin $(git branch --show-current) 2>&1
```

Report the result. If the push fails, **stop here** -- do not post any replies until
the push succeeds. Explain why the push failed; do not retry automatically.

---

## Step 8 -- Post replies

Only after a successful push, post all replies in one batch (do not wait between them):

```bash
bash ~/.claude/scripts/reply-comment.sh "$pr_number" {COMMENT_ID} '<reply text>'
```

Run one call per open comment. Log each one as it completes.

---

## Step 8.5 -- Resolve review threads

After a successful push, **always** resolve review threads that were replied to in
this round. This removes visual clutter from the PR conversation and signals that
feedback has been addressed.

### Copilot threads -- GraphQL resolve

Pass all Copilot comment IDs that were replied to in this round:

```bash
bash ~/.claude/scripts/resolve-threads.sh "$pr_number" 123456 789012 ...
```

### CodeRabbit threads -- `@coderabbitai resolve`

Do not use the GraphQL resolve mutation for CodeRabbit threads. Instead, post a single
PR-level comment to resolve all addressed CR threads at once:

```bash
gh pr comment "$pr_number" --body "@coderabbitai resolve"
```

This tells CodeRabbit to mark all its threads that have been replied to as resolved.
Only post this after all replies to CR comments have been posted in Step 8.

Report how many Copilot threads were resolved and that CR resolve was requested.

---

## Step 9 -- Summary and Copilot re-review recommendation

**Build the summary deterministically from tracked variables.** Do not write it
freehand. Compute each field from the data already collected in earlier steps:

| Field | Source |
|-------|--------|
| PR link | `$repo` (Step 1) + `$pr_number` (Step 1) |
| Fixed count | `count(triage where category in {bug, test-gap})` |
| Dismissed count | `count(triage where category in {false-positive, wont-fix})` |
| Noted count | `count(triage where category == already-fixed)` |
| Replied count | total open comments from Step 3 |
| SHA | `git rev-parse --short HEAD` from Step 7 |
| Branch | `git branch --show-current` |
| Resolved | Copilot thread count from Step 8.5 + whether CR resolve was posted |

Assemble and print:

```
## Done -- PR [#$pr_number](https://github.com/$repo/pull/$pr_number)

- Fixed: $fixed_count comments (bug/test)
- Dismissed: $dismissed_count comments (false-positive/wont-fix)
- Noted: $noted_count comments (already-fixed)
- Replied: $replied_count total
- Pushed: $sha to $branch
- Resolved: $copilot_resolved Copilot threads, CR resolve $cr_status
```

### Copilot re-review assessment

Copilot's automatic re-review on push is disabled (too noisy -- 61% of later-round
comments are repeats). After pushing fixes, assess whether a manual Copilot re-review
is warranted based on the scope of changes:

**Recommend re-review** when fixes touched:
- Concurrency primitives or shared state
- Error handling paths (new error branches, changed return values)
- Security-sensitive code (token handling, file paths)
- Substantial new code (not just one-line fixes)

**Skip re-review** when fixes were:
- Comment/doc-only changes
- Trivial one-line corrections (typo, off-by-one, missing nil check)
- Test-only changes
- Style/formatting fixes

Print the recommendation:

If re-review is warranted:
```
Copilot re-review recommended -- fixes touched [category]. Trigger manually from
the GitHub PR page (re-request review from Copilot). The API does not support
re-requesting review from bot accounts.
```

If re-review is not warranted:
```
Copilot re-review not needed -- fixes were minor ([category]).
CodeRabbit will review the push automatically.
```
