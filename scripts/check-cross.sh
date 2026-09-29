#!/usr/bin/env bash
set -euo pipefail
source "$(dirname -- "$0")/env.sh"

# Compile recovery/storage tests even when their packages are not linked into
# installed services yet. These binaries are not executed on the build host.
mkdir -p .cache/cross-tests
for DUCKOS_TEST_ARCH in amd64 arm64 arm; do
  for DUCKOS_TEST_PACKAGE in network vault catalog; do
    CGO_ENABLED=0 GOOS=linux GOARCH="$DUCKOS_TEST_ARCH" GOARM=7 \
      go test -c -o ".cache/cross-tests/${DUCKOS_TEST_PACKAGE}-linux-${DUCKOS_TEST_ARCH}.test" "./internal/${DUCKOS_TEST_PACKAGE}"
  done
done
printf 'Linux amd64/arm64/ARMv7 recovery and storage test binaries compiled; native execution is separate.\n'
