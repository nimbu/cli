#!/usr/bin/env bash
set -euo pipefail

root="${1:-.claude/skills}"
failed=0

echo "Checking Nimbu skill doc links in ${root}"

if rg -n 'docs\.nimbu\.io/(themes|cloud-code|sdk|api)(/|$)' "${root}" -g '*.md'; then
  echo "Found root-mounted docs.nimbu.io links. Use https://docs.nimbu.io/docs/<section>/..."
  failed=1
fi

while IFS= read -r skill_file; do
  if ! sed -n '1,/^---$/p' "${skill_file}" | rg -q '^[[:space:]]+version: "[0-9]+\.[0-9]+\.[0-9]+"$'; then
    echo "Missing metadata.version semver in ${skill_file}"
    failed=1
  fi
done < <(find "${root}" -name SKILL.md -path '*/nimbu*/*' -print)

mapfile -t urls < <(
  rg --no-filename -o 'https://docs\.nimbu\.io[^\s)<>]+' "${root}" -g '*.md' \
    | sed 's/[.,;:]*$//' \
    | sort -u
)

for url in "${urls[@]}"; do
  if ! curl -fsSIL --max-time 20 --retry 2 "${url}" >/dev/null; then
    echo "Broken link: ${url}"
    failed=1
  fi
done

if [[ "${failed}" -ne 0 ]]; then
  exit 1
fi

echo "Checked ${#urls[@]} docs.nimbu.io links"
