#!/usr/bin/env bash
set -euo pipefail

root="${1:-.claude/skills}"
failed=0

declare -a stale_patterns=(
  'nimbu themes serve'
  'nimbu translations index'
  'nimbu translations push'
  'nimbu translations pull'
  'nimbu apps:code:'
  '--write'
)

for pattern in "${stale_patterns[@]}"; do
  if rg -n -F -g '*.md' -- "${pattern}" "${root}"; then
    echo "Stale Nimbu command example: ${pattern}"
    failed=1
  fi
done

if rg -n -P -g '*.md' -- 'nimbu jobs run\s+(?!--)[A-Za-z0-9_]' "${root}"; then
  echo "Stale positional jobs run example; pass the job with --job"
  failed=1
fi

if rg -n -P -g '*.md' -- 'nimbu jobs run[^\n]*--app(?:=|\s)' "${root}"; then
  echo "jobs run must resolve its owning app from the site-level job registry"
  failed=1
fi

contract="$(mktemp)"
trap 'rm -f "${contract}"' EXIT
go run ./cmd/nimbu-cli commands --json >"${contract}"

declare -a required_commands=(
  'nimbu api get'
  'nimbu products attachments download'
  'nimbu uploads download'
  'nimbu apps code get'
  'nimbu pages versions restore'
  'nimbu customers roles set'
  'nimbu settings consent list'
)

for command in "${required_commands[@]}"; do
  if ! jq -e --arg command "${command}" '.commands[] | select(.path == $command)' "${contract}" >/dev/null; then
    echo "Required skill command missing from CLI contract: ${command}"
    failed=1
  fi
done

exit "${failed}"
