#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

run_lint() {
  if command -v markdownlint-cli2 >/dev/null 2>&1; then
    markdownlint-cli2 "$@"
  else
    npx --yes markdownlint-cli2 "$@"
  fi
}

if [[ $# -gt 0 ]]; then
  run_lint "$@"
else
  run_lint \
    "docs/**/*.md" \
    "README.md" \
    "RELEASES.md" \
    "extras/**/README.md" \
    ".github/CONTRIBUTING.md"
fi
