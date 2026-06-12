#!/usr/bin/env bash
set -euo pipefail

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

mkdir -p "$tmp_dir/scripts/ci" "$tmp_dir/config"
cp "$repo_root/scripts/ci/check-no-sensitive-links.sh" "$tmp_dir/scripts/ci/check-no-sensitive-links.sh"

(
  cd "$tmp_dir"
  git init -q
  git config user.name test
  git config user.email test@example.com

  cat <<'EOF' > config/values.example.yaml
proxy-providers:
  example:
    type: http
    url: https://example.com/subscription.yaml
EOF

  git add config/values.example.yaml scripts/ci/check-no-sensitive-links.sh
  bash scripts/ci/check-no-sensitive-links.sh
)

(
  cd "$tmp_dir"
  cat <<'EOF' > config/values.example.yaml
proxy-providers:
  private:
    type: http
    url: https://provider.example/private-token
EOF

  git add config/values.example.yaml
  if bash scripts/ci/check-no-sensitive-links.sh >/dev/null 2>&1; then
    echo "expected sensitive-link check to fail for non-placeholder provider URLs"
    exit 1
  fi
)
