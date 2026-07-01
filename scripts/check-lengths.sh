#!/usr/bin/env bash
set -euo pipefail

STAGED=$(git diff --cached --name-only --diff-filter=ACM)
[ -z "$STAGED" ] && exit 0

EXIT=0
for f in $STAGED; do
  [ ! -f "$f" ] && continue
  LINES=$(wc -l < "$f")
  case "$f" in
    *.go)       MAX=500 ;;
    *.ts)       MAX=300 ;;
    *.vue)      MAX=400 ;;
    *)          continue ;;
  esac
  if [ "$LINES" -gt "$MAX" ]; then
    echo "BLOCKED: $f has $LINES lines (max $MAX)"
    EXIT=1
  fi
done
exit $EXIT
