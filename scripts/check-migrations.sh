#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
fail=0

echo "=== Migration sequence check ==="

prev=0
for up in migrations/*.up.sql; do
  base=$(basename "$up" .up.sql)
  num=$((10#${base%%_*}))
  down="migrations/${base}.down.sql"

  if [ ! -f "$down" ]; then
    echo "FAIL: missing $down"
    fail=1
  fi

  expected=$((prev + 1))
  if [ "$num" -ne "$expected" ]; then
    echo "FAIL: gap after 000$(printf '%03d' "$prev"), found $num"
    fail=1
  fi
  prev=$num
done

if [ "$fail" -eq 0 ]; then
  echo "PASS: all $prev migrations contiguous with matching down files"
fi

exit $fail
