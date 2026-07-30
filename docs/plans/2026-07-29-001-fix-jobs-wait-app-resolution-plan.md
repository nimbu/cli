---
title: "fix: resolve job log ownership from the site"
type: fix
status: active
date: 2026-07-29
---

# Fix job log ownership resolution

## Summary

Make `nimbu jobs run --wait` work from any directory with only a site and globally unique job name. The CLI will resolve the owning app internally from the site's registered jobs before it schedules the job.

## Requirements

- R1. `jobs run --wait` must not depend on `nimbu.yml` or the current directory.
- R2. Callers must not need to know or pass an app name or key.
- R3. App ownership must be resolved before scheduling so discovery failures cannot leave a running job behind a failed command.
- R4. Inline assignments and `--file=-` must keep sending a flat params body.
- R5. CLI contracts, skills, and public docs must describe the same behavior.

## Scope boundaries

- Keep `apps logs` app-specific because it tails an app rather than one globally unique job.
- Do not change the server API or job scheduling response.
- Do not add persistent app configuration outside project files.

## Key technical decisions

- Reuse the site's registered app/job data already consumed by `jobs list`; the job name selects exactly one owning app.
- Treat zero matches as an unregistered job and multiple matches as a violated server invariant.
- Perform discovery only for `--wait`; non-waiting job execution keeps its current single request.
- Remove `--app` from `jobs run` rather than documenting an internal workaround.

## Implementation units

### U1. Cover the site-level job contract

**Goal:** Prove the command works outside a project directory and fails safely when ownership cannot be resolved.

**Files:**
- Modify: `internal/cmd/jobs_test.go`
- Modify: `internal/cmd/cli_grammar_test.go`

**Execution note:** Start with failing integration-style command tests.

**Test scenarios:**
- Happy path: one site app registers the requested job; `--wait` schedules it and tails that app's filtered logs without local config.
- Error path: no app registers the job; the command does not send the scheduling request.
- Error path: multiple apps register the same job; the command reports the invariant violation and does not schedule.
- Compatibility: typed inline params remain flat in the scheduling body.
- Contract: `jobs run` no longer exposes `--app`.

### U2. Resolve ownership before scheduling

**Goal:** Hide the app-log endpoint detail behind the site-level jobs command.

**Dependencies:** U1

**Files:**
- Modify: `internal/cmd/jobs_run.go`
- Modify: `internal/cmd/jobs_list.go`

**Approach:**
- Share the existing registered-job loading path between listing and ownership resolution.
- Resolve one app key before posting the job when `--wait` is enabled.
- Pass the resolved key only to the internal log-tail implementation.

**Verification:**
- The U1 tests pass without consulting `nimbu.yml`.
- Existing jobs list, jobs run, and app logs tests remain green.

### U3. Align agent guidance and docs

**Goal:** Remove stale positional and app-dependent examples from every maintained surface.

**Dependencies:** U2

**Files:**
- Modify: `.claude/skills/nimbu-cloud-code/SKILL.md`
- Modify: `.claude/skills/nimbu-cloud-code/references/sdk-cheatsheet.md`
- Modify: generated CLI command contract in the companion `nimbu-docs` repository
- Modify: cloud-code jobs documentation in the companion `nimbu-docs` repository

**Test scenarios:**
- Generated command reference contains no `--app` flag for `jobs run`.
- Skill command checks accept the updated examples.
- Docs build succeeds from the refreshed contract.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Discovery adds requests before a waiting job run | Restrict discovery to `--wait` and reuse the same app detail calls required by `jobs list`. |
| A stale job registry blocks a valid run | Return the existing registration guidance before scheduling; non-waiting runs remain unaffected. |
| Duplicate job registrations violate the uniqueness rule | Fail before scheduling and name the conflicting app keys. |

## Verification

- Targeted jobs and command-contract tests.
- Full Go test suite, lint, and build.
- Skill command and link checks.
- Deterministic command-contract refresh and full docs build.
- GitHub Actions and incoming review feedback on the linked CLI and docs PRs.
