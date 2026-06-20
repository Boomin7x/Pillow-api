#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

unit=/tmp/cover-unit.out
intg=/tmp/cover-intg.out

# Unit coverage for domain/service/handler.
go test -coverpkg=./internal/... -coverprofile="$unit" ./internal/domain/... ./internal/kyc/... > /dev/null

# Repository is exercised only by the integration suite (real Postgres), so its
# coverage must be measured with the integration build tag (Rule F4 / 8.17.1).
go test -tags=integration -coverpkg=./internal/kyc/...,./internal/infrastructure/... -coverprofile="$intg" ./test/integration/... > /dev/null

avg() {
  go tool cover -func="$1" | awk -v pat="$2" '$0 ~ pat {gsub(/%/,"",$NF); s+=$NF; c++} END {if (c>0) printf "%.1f", s/c; else print "NaN"}'
}

check() {
  local profile="$1" pattern="$2" threshold="$3" label="$4" pct
  pct=$(avg "$profile" "$pattern")
  if [ "$pct" = "NaN" ]; then
    echo "FAIL: $label - no functions matched $pattern"
    exit 1
  fi
  if awk "BEGIN{exit !($pct < $threshold)}"; then
    echo "FAIL: $label avg coverage = ${pct}%, threshold = ${threshold}%"
    exit 1
  fi
  echo "PASS: $label avg coverage = ${pct}% (>= ${threshold}%)"
}

check "$unit" "internal/domain/kyc.go"    100 "domain"
check "$unit" "internal/kyc/service.go"    90 "service"
check "$unit" "internal/kyc/handler.go"    70 "handler"
check "$intg" "internal/kyc/repository.go" 60 "repository"

rm -f "$unit" "$intg"
