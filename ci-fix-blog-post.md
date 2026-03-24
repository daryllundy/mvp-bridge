# Why GitHub Kept Failing My PRs When Local Tests Passed

I ran into a pattern that is common in small Go repos as they grow: feature branches looked fine locally because `go test ./...` passed, but GitHub Actions kept failing the pull request on lint and security checks.

This post explains what was actually happening in MVPBridge, why it kept recurring on new feature branches, and what I changed to stop it.

## The Symptom

The PR checks that kept failing were:

- `Lint`
- `Security Scan`

The confusing part was that the failures often did not look directly tied to the feature I had just built. A PR could add one new subsystem, and GitHub would suddenly complain about:

- duplicate string literals in an older file
- function complexity in a new helper
- file-path handling in a newly added local copy routine

Meanwhile, local testing looked green:

```bash
go test ./...
```

That mismatch made the CI failures feel random. They were not random.

## The Root Cause: GitHub Lints the PR Merge State

The first important detail is that GitHub Actions was not linting “just the files I changed.” It was linting the full pull request merge ref.

That means CI evaluated the state of:

- my feature branch
- plus the base branch
- plus the merged result GitHub would create if the PR were accepted

That matters because repo-wide tools see the whole codebase at once.

For example, `golangci-lint` was running:

```bash
golangci-lint run --timeout=5m
```

And `gosec` was running:

```bash
gosec -exclude=G704 ./...
```

Both commands analyze the repository broadly, not just a narrow diff.

## Why Old Files Started Failing After New Code Was Added

One of the lint failures came from `goconst`. That linter does not care only about whether a single string is duplicated in one file. It looks at repeated literals and decides when the duplication crosses a threshold.

In this repo, repeated defaults like these were the trigger:

- `npm run build`
- `dist`

Those strings already existed in the codebase. The new feature added more fallback paths using the same literals. Once the total occurrence count crossed the threshold, `goconst` started reporting findings in places that were not the main focus of the new feature.

So the practical lesson is:

- a new PR can make an older file fail lint without that older file being conceptually “broken”
- threshold-based linters operate on the aggregate codebase, not just your latest diff

## Why Complexity Warnings Showed Up

Two new functions also grew large enough to trip `gocyclo`:

- the persistence inference logic
- the local deploy workspace generator

Neither function was wrong from a behavior standpoint. They had just accumulated too many branches and responsibilities in one place.

That is exactly the kind of signal complexity linting is meant to provide:

- a function is doing more than one job
- future edits will become riskier
- tests may still pass, but maintainability is already slipping

The fix was not to weaken the rule. The fix was to split the logic by responsibility.

## Why `gosec` Flagged the Local Deploy Generator

The local deploy feature added a snapshot-copy path that opened source files and created destination files dynamically.

Even though the code was walking a known repo tree, `gosec` correctly treated those variable paths as suspicious until the code proved they were rooted and validated.

The initial issue was not that the code was definitely vulnerable. The issue was that the safety properties were not explicit enough for either:

- the reviewer
- or the static analyzer

To fix that, I changed the copy flow so it only works with validated relative paths under trusted roots.

The new structure is:

- walk the trusted source root
- compute a relative path from that walk
- reject empty paths
- reject absolute paths
- reject `..` traversal
- derive source and destination from known roots
- then perform the copy

That made the code safer and easier to explain. Where `gosec` still flagged variable path operations, I kept narrow `#nosec` annotations directly on those lines with the reason documented inline.

## What Changed in the Code

I made three categories of fixes.

### 1. I extracted helper functions to reduce complexity

The persistence detection logic was split into smaller helpers for:

- dependency-based scoring
- file-content-based scoring
- final persistence resolution

The local deploy workspace generation was split into helpers for:

- resolving the workspace root
- initializing the generated directory
- normalizing defaults
- writing generated assets
- building the final result

This kept behavior the same, but reduced cyclomatic complexity enough for lint to pass cleanly.

### 2. I centralized fallback defaults

The repeated default values that triggered `goconst` were moved into shared constants:

- default build command
- default output directory

That fixed the immediate lint issue and made the defaults easier to reuse consistently.

### 3. I tightened file-copy path handling

The local deploy generator now copies files through a rooted relative-path helper rather than by accepting arbitrary full paths.

That means the code now makes its safety model explicit:

- source files must stay under the source root
- destination files must stay under the generated workspace
- traversal and absolute paths are rejected before file operations happen

This fixed the `gosec` findings without muting the CI job.

## The Practical Workflow Change

The real workflow lesson is simple:

`go test ./...` is necessary, but it is not CI parity.

For this repo, CI parity means running all three commands locally before pushing:

```bash
go test ./...
golangci-lint run --timeout=5m
gosec -exclude=G704 ./...
```

Once I started treating lint and security checks as part of local development instead of “GitHub’s problem,” the failures stopped feeling random and started looking like normal engineering feedback.

## The Bigger Takeaway

A PR that fails lint or security is not always telling you that your latest feature is bad. Sometimes it is telling you something more subtle:

- the repo has reached a threshold where duplication is visible
- a helper absorbed too many responsibilities
- a security-sensitive operation is correct in spirit but not explicit enough in implementation

That is exactly why these checks are useful.

They do not just catch broken code. They also catch the point where a codebase starts becoming harder to reason about.

In this case, the fix was not to relax GitHub Actions. The fix was to make the code clearer, safer, and more intentional.
