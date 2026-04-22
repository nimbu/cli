#!/usr/bin/env bash
set -euo pipefail

source_site="${1:-zenjoy2024}"
channel="${2:-project_approaches}"
entry_slug="${3:-start}"
page="${4:-home}"
menu="${5:-main}"
out_dir="internal/testdata/nimbu_api/zenjoy_get_raw"

mkdir -p "$out_dir"

scrub_filter='
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
'

capture() {
  local site="$1"
  local path="$2"
  local output="$3"

  nimbu api GET "$path" --site "$site" --json | jq "$scrub_filter" > "$out_dir/$output"
}

capture "$source_site" "/channels/${channel}" "channel_${channel}.raw.json"
capture "$source_site" "/channels" "channels.raw.json"
capture "$source_site" "/channels/${channel}/entries?where=_slug:%22${entry_slug}%22" "entries_${channel}_${entry_slug}_default.raw.json"
capture "$source_site" "/channels/${channel}/entries?where=_slug:%22${entry_slug}%22&content_locale=en" "entries_${channel}_${entry_slug}_en.raw.json"
capture "$source_site" "/pages/${page}?locale=en" "page_${page}_en.raw.json"
capture "$source_site" "/menus/${menu}" "menu_${menu}.raw.json"

printf 'Wrote scrubbed raw GET cassettes to %s\n' "$out_dir"
printf 'Review and normalize fixture IDs/content before moving selected files into committed cassette directories.\n'
