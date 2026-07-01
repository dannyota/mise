#!/usr/bin/env bash
set -euo pipefail

# Exclude this script itself: its own pattern definitions (below) read as
# false positives against themselves once staged for the first time.
STAGED=$(git diff --cached --name-only --diff-filter=ACM -- . ':(exclude)scripts/check-leaks.sh')
[ -z "$STAGED" ] && exit 0

# Case-sensitive: real tokens have fixed casing (AWS/GitHub prefixes are
# never lowercased), so -i buys nothing but false positives against
# incidental base64 (e.g. lockfile integrity hashes).
DENY_PATTERNS='BEGIN.*PRIVATE KEY|service_account.*\.json|AKIA[A-Z0-9]{16}|ghp_[a-zA-Z0-9]{36}|ghs_[a-zA-Z0-9]{36}'
ALLOW_PATTERNS='your-|000000|placeholder|example'

MATCHES=$(git diff --cached -U0 -- $STAGED | grep -E "$DENY_PATTERNS" | grep -ivE "$ALLOW_PATTERNS" || true)

if [ -n "$MATCHES" ]; then
  echo "BLOCKED: potential secret in staged diff:"
  echo "$MATCHES"
  exit 1
fi
