#!/usr/bin/env bash
set -euo pipefail

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$repo_root"

provider_url_matches="$(
  awk '
    FILENAME == "config/values.example.yaml" {
      if ($1 == "url:" && $2 !~ /^https:\/\/example\.com\//) {
        printf "%s:%d:%s\n", FILENAME, NR, $0
      }
    }
  ' config/values.example.yaml
)"

if [ -n "$provider_url_matches" ]; then
  echo "Sensitive provider links detected in tracked files."
  printf '%s\n' "$provider_url_matches"
  echo
  echo "Keep tracked provider examples on example.com only, and keep real provider links in local-only config/values.yaml."
  exit 1
fi
