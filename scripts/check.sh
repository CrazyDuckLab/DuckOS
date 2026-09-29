#!/usr/bin/env bash
set -euo pipefail
source "$(dirname -- "$0")/env.sh"
UNFORMATTED=$(gofmt -l cmd internal pkg packaging tests)
[[ -z "$UNFORMATTED" ]] || { printf 'Go formatting required:\n%s\n' "$UNFORMATTED" >&2; exit 1; }
node scripts/schema.mjs --check
go mod verify
go vet ./...
go test -race -count=1 ./...
npm --prefix web run typecheck
npm --prefix web run format:check
npm --prefix web test
node --test tests/installer/*.test.mjs
node --test tests/documentation/*.test.mjs
for SCRIPT in scripts/*.sh; do bash -n "$SCRIPT"; done
git diff --check
printf 'Checks passed on %s/%s. Linux-only tests skip explicitly on non-Linux hosts.\n' "$(uname -s)" "$(uname -m)"
