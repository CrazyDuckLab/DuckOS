# Sourced by repository scripts. Paths are local to the checkout.
ROOT=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
export PATH="$ROOT/.tools/go/bin:$ROOT/.tools/node/bin:$PATH"
export GOTOOLCHAIN=local
export GOCACHE="$ROOT/.cache/go-build"
export GOMODCACHE="$ROOT/.cache/go-mod"
export npm_config_cache="$ROOT/.cache/npm"
cd "$ROOT"
[[ $(go env GOVERSION) == go1.27.1 ]] || { printf '%s\n' 'Run bash scripts/bootstrap.sh: Go 1.27.1 required.' >&2; exit 1; }
[[ $(node --version) == v24.20.0 ]] || { printf '%s\n' 'Run bash scripts/bootstrap.sh: Node 24.20.0 required.' >&2; exit 1; }
