# version.sh: deterministic build version identity.
#
# Usage (sourced): VERSION=$(bash scripts/version.sh "$COMMIT" "$DIRTY")
# Environment: DUCKOS_RELEASE_VERSION (optional official release tag).
#
# Development builds keep the historical 1.0.0-dev.<commit> identity
# (plus .dirty). Official release builds require DUCKOS_RELEASE_VERSION
# to be a valid semver tag (vMAJOR.MINOR.PATCH[-PRERELEASE]), the tree
# to be clean, and — when a git tag of that name exists — the tag to
# point at exactly the source commit being built. Any mismatch fails
# the build; the release identity can never silently describe sources
# it was not produced from.
set -euo pipefail

COMMIT=${1:?source commit required}
DIRTY=${2:?dirty flag required}
RELEASE=${DUCKOS_RELEASE_VERSION:-}

if [[ -z "$RELEASE" ]]; then
	VERSION="1.0.0-dev.${COMMIT:0:12}"
	[[ "$DIRTY" == false ]] || VERSION="$VERSION.dirty"
	printf '%s\n' "$VERSION"
	exit 0
fi

if [[ ! "$RELEASE" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$ ]]; then
	printf 'release version %q is not a valid semver tag\n' "$RELEASE" >&2
	exit 1
fi

if [[ "$RELEASE" == *-* ]]; then
	IFS=. read -r -a RELEASE_PARTS <<< "${RELEASE#*-}"
	for PART in "${RELEASE_PARTS[@]}"; do
		if [[ "$PART" =~ ^[0-9]+$ && ${#PART} -gt 1 && "$PART" == 0* ]]; then
			printf 'numeric prerelease identifier %q has a leading zero\n' "$PART" >&2
			exit 1
		fi
	done
fi

if [[ "$DIRTY" != false ]]; then
	printf '%s\n' 'release builds require a clean working tree' >&2
	exit 1
fi

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
if TAG_COMMIT=$(git -C "$SCRIPT_DIR/.." rev-parse -q --verify "refs/tags/$RELEASE^{commit}" 2>/dev/null); then
	if [[ "$TAG_COMMIT" != "$COMMIT" ]]; then
		printf 'tag %s points at %s, refusing to stamp %s\n' "$RELEASE" "${TAG_COMMIT:0:12}" "${COMMIT:0:12}" >&2
		exit 1
	fi
else
	printf 'release version %s has no git tag at the source commit; create it before building\n' "$RELEASE" >&2
	exit 1
fi

printf '%s\n' "$RELEASE"
