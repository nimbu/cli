#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 4 ]]; then
  printf 'usage: %s <source-site> <target-site> <channel> <slug>\n' "$0" >&2
  exit 2
fi

source_site="$1"
target_site="$2"
channel="$3"
slug="$4"
out_dir="internal/testdata/nimbu_api/localized_${channel}"

mkdir -p "$out_dir"

capture() {
  local site="$1"
  local path="$2"
  local output="$3"

  nimbu api GET "$path" --site "$site" --json |
    jq '
      def scrub:
        walk(
          if type == "object" then
            with_entries(
              if (.key | test("token|secret|password|authorization"; "i")) then
                .value = "fixture-redacted"
              else
                .
              end
            )
          elif type == "string" and test("^[0-9a-f]{24}$") then
            "fixture-id"
          else
            .
          end
        );
      scrub
    ' > "$out_dir/$output"
}

capture "$source_site" "/sites/${source_site}/settings" "source_settings.raw.json"
capture "$target_site" "/sites/${target_site}/settings" "target_settings.raw.json"
capture "$source_site" "/channels" "channels.raw.json"
capture "$source_site" "/channels/${channel}/entries?where=_slug:%22${slug}%22" "source_entries_nl.raw.json"
capture "$source_site" "/channels/${channel}/entries?where=_slug:%22${slug}%22&content_locale=en" "source_entries_en.raw.json"
capture "$target_site" "/channels/${channel}/entries?where=_slug:%22${slug}%22" "target_entries_nl.raw.json"
capture "$target_site" "/channels/${channel}/entries?where=_slug:%22${slug}%22&content_locale=en" "target_entries_en.raw.json"

printf 'Wrote scrubbed raw cassettes to %s\n' "$out_dir"
printf 'Review and normalize fixture-id values before committing so source/target IDs stay distinct and deterministic.\n'
