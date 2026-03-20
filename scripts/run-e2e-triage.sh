#!/usr/bin/env bash

set -euo pipefail

: "${RUN_URL:?RUN_URL is required}"
: "${E2E_AGENT:?E2E_AGENT is required}"
: "${TRIAGE_OUTPUT_FILE:?TRIAGE_OUTPUT_FILE is required}"

mkdir -p "$(dirname "$TRIAGE_OUTPUT_FILE")"

# Download artifacts before invoking Claude so it only needs read-only access
artifact_path="$(scripts/download-e2e-artifacts.sh "$RUN_URL")"

triage_args="/e2e:triage-ci ${artifact_path} --agent ${E2E_AGENT}"
if [ -n "${TRIAGE_SHA:-}" ]; then
  triage_args="${triage_args} --sha ${TRIAGE_SHA}"
fi

claude \
  --plugin-dir .claude/plugins/e2e \
  --output-format text \
  --allowedTools \
    "Read" \
    "Grep" \
    "Glob" \
  -p "$triage_args" \
  2>&1 | sed 's/\x1b\[[0-9;]*[a-zA-Z]//g; s/\x1b\[[?][0-9]*[a-zA-Z]//g' \
  | tee "$TRIAGE_OUTPUT_FILE"
