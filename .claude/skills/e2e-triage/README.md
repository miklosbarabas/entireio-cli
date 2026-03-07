# E2E Triage Skill

Triage E2E test failures by re-running tests locally, classifying failures as flaky vs real-bug, and applying fixes interactively.

## Usage

```
# Triage a specific test
/e2e-triage TestInteractiveMultiStep

# Triage a specific test for one agent
/e2e-triage TestInteractiveMultiStep --agent claude-code

# Triage multiple tests
/e2e-triage TestInteractiveMultiStep TestCheckpointRewind

# Analyze existing artifacts (skip re-running)
/e2e-triage /path/to/artifact/dir

# Download latest CI run artifacts and triage
/e2e-triage get latest CI run
```

The skill will:
1. Run the test(s) up to 3 times (first run, re-run on failure, tiebreaker if split)
2. Analyze artifacts (`console.log`, `entire.log`, `git-log.txt`, checkpoint metadata)
3. Classify each failure and present findings
4. Ask before applying any fixes

## Classification Logic

Re-run results are the primary signal:

| Pattern | Classification |
|---------|---------------|
| FAIL / PASS / PASS | Flaky |
| FAIL / PASS / FAIL | Flaky (non-deterministic) |
| FAIL / FAIL / PASS | Flaky (non-deterministic) |
| FAIL / FAIL / FAIL | Real-bug OR flaky (test-bug) — depends on root cause location |

**Key distinction for consistent failures:** if the root cause is in `cmd/entire/cli/` (product code), it's a **real-bug**. If it's in `e2e/` (test infra), it's **flaky (test-bug)**.

## Related Skills

- `/debug-e2e` — Standalone artifact analysis for diagnosing a specific failure. Use when you already have artifacts and want to understand *what went wrong* without re-running or classifying.
- `/e2e-triage` uses debug-e2e's workflow internally for its analysis step, then adds classification (flaky vs real-bug) and automated action.

## Key Files

- `SKILL.md` — Full skill definition with all steps, classification rules, and action templates
- `../../scripts/download-e2e-artifacts.sh` — Downloads artifacts from CI runs
