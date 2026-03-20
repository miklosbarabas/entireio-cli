#!/usr/bin/env bash

set -euo pipefail

: "${RUN_URL:?RUN_URL is required}"
: "${E2E_AGENT:?E2E_AGENT is required}"
: "${TRIAGE_OUTPUT_FILE:?TRIAGE_OUTPUT_FILE is required}"

mkdir -p "$(dirname "$TRIAGE_OUTPUT_FILE")"

triage_args="/e2e:triage-ci ${RUN_URL} --agent ${E2E_AGENT}"
if [ -n "${TRIAGE_SHA:-}" ]; then
  triage_args="${triage_args} --sha ${TRIAGE_SHA}"
fi

claude --plugin-dir .claude/plugins/e2e \
  -p "$triage_args" \
  2>&1 | tee "$TRIAGE_OUTPUT_FILE"
