SHELL := /bin/bash
.PHONY: bootstrap deps format check check-cross test build build-all
bootstrap:
	bash scripts/bootstrap.sh
deps:
	bash -c 'set -euo pipefail; source scripts/env.sh; go mod download; go mod verify; npm --prefix web ci'
format:
	bash -c 'set -euo pipefail; source scripts/env.sh; gofmt -w cmd internal pkg packaging tests; node scripts/schema.mjs; npm --prefix web run format'
check test:
	bash scripts/check.sh
check-cross:
	bash scripts/check-cross.sh
build:
	bash scripts/build.sh amd64
build-all:
	bash scripts/build.sh all
