#!/usr/bin/env bash
# PostToolUse hook (Write|Edit): gofmt -w + go vet on the touched .go file's package.
# Reads the standard Claude Code hook JSON from stdin.
set -u

input="$(cat)"
file="$(printf '%s' "$input" | jq -r '.tool_response.filePath // .tool_input.file_path // empty')"

if [[ -z "$file" || "$file" != *.go ]]; then
  exit 0
fi
if [[ ! -f "$file" ]]; then
  exit 0
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root" || exit 0

gofmt -w "$file" 2>/dev/null

pkg_dir="$(dirname "$file")"
case "$pkg_dir" in
  "$repo_root") rel_dir="." ;;
  "$repo_root"/*) rel_dir="${pkg_dir#"$repo_root"/}" ;;
  /*) rel_dir="$pkg_dir" ;;
  *) rel_dir="$pkg_dir" ;;
esac
if [[ "$rel_dir" = /* ]]; then
  vet_output="$(go vet "$rel_dir" 2>&1)"
else
  vet_output="$(go vet "./$rel_dir" 2>&1)"
fi
vet_status=$?

if [[ $vet_status -ne 0 ]]; then
  echo "go vet found issues in $pkg_dir after editing $file:" >&2
  echo "$vet_output" >&2
  exit 2
fi

exit 0
