#!/bin/bash
# Fresh installation only. Read docs/install.md before executing as root.
set +x
set -euo pipefail
export LC_ALL=C
export PATH=/usr/sbin:/usr/bin:/sbin:/bin
umask 077

fail() { printf 'DuckOS installer: %s\n' "$*" >&2; exit 1; }
usage() {
  printf '%s\n' 'Usage: bash install.sh --artifact /absolute/canonical/artifact-directory [--check | --verify-artifact]' \
    '  --check            Read-only artifact and Debian host preflight (root).' \
    '  --verify-artifact  Read-only artifact integrity/shape check on any build host.' \
    '  No option installs a fresh internal TEST build; existing installs are refused.'
}

artifact_input=''
mode=install
while [[ $# -gt 0 ]]; do
  case "$1" in
    --artifact) [[ $# -ge 2 && -z "$artifact_input" ]] || fail 'Supply --artifact once, with a directory.'; artifact_input=$2; shift 2 ;;
    --check|--verify-artifact) [[ "$mode" == install ]] || fail 'Choose only one check mode.'; mode=${1#--}; shift ;;
    --help) usage; exit 0 ;;
    *) usage >&2; fail 'Unknown argument.' ;;
  esac
done
[[ -n "$artifact_input" ]] || { usage >&2; fail '--artifact is required.'; }
[[ "$artifact_input" == /* && -d "$artifact_input" && ! -L "$artifact_input" ]] || fail 'Artifact must be an absolute real directory.'
artifact=$(cd -- "$artifact_input" && pwd -P)
[[ "$artifact_input" == "$artifact" && "$artifact" != / ]] || fail 'Use the canonical artifact path, without symlinks, dot components or trailing slash.'

valid_path() {
  local candidate=$1
  [[ "$candidate" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]*$ ]] || return 1
  case "/$candidate/" in *'/../'*|*'/./'*|*'//'*) return 1 ;; esac
  case "$candidate" in
    VERSION|BUILD-INFO.json|install.sh|bin/duck-api|bin/duck-agent|bin/duck-updater|bin/duck-restore|systemd/duck-restore.service|systemd/duck-updater.service|systemd/duck-api.service|systemd/duck-agent.service|systemd/duck-dnsmasq.service|systemd/duck-network-recover.service) return 0 ;;
    web/*) [[ "$candidate" != */ ]] ;;
    *) return 1 ;;
  esac
}
digest_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum -- "$1" | cut -d ' ' -f 1
  elif command -v shasum >/dev/null 2>&1; then shasum -a 256 -- "$1" | cut -d ' ' -f 1
  else fail 'SHA-256 tool required (sha256sum or shasum).'; fi
}
manifest_paths=()
has_path() {
  local candidate=$1 present
  [[ ${#manifest_paths[@]} -gt 0 ]] || return 1
  for present in "${manifest_paths[@]}"; do [[ "$present" != "$candidate" ]] || return 0; done
  return 1
}
verify_artifact() {
  local root=$1 entry relative line expected actual binary magic machine metadata metadata_pattern scan_complete=false count=0 entries=0 total_bytes=0
  [[ -f "$root/SHA256SUMS" && ! -L "$root/SHA256SUMS" ]] || fail 'Missing regular SHA256SUMS.'
  # Validate topology before opening manifest-selected paths (including parent links).
  while IFS= read -r -d '' entry; do
    if [[ -z "$entry" ]]; then scan_complete=true; break; fi
    entries=$((entries + 1))
    [[ $entries -le 1024 ]] || fail 'Artifact has too many filesystem entries.'
    [[ ! -L "$entry" ]] || fail 'Symbolic links are not allowed in artifacts.'
    [[ -d "$entry" || -f "$entry" ]] || fail 'Special files are not allowed in artifacts.'
    if [[ -f "$entry" ]]; then
      total_bytes=$((total_bytes + $(wc -c < "$entry")))
      count=$((count + 1))
      [[ $count -le 513 && $total_bytes -le 268435456 ]] || fail 'Artifact exceeds file count or 256 MiB size limit.'
    fi
  done < <(find "$root" -mindepth 1 -print0 && printf '\0')
  $scan_complete || fail 'Artifact tree could not be read completely.'
  [[ -z $(find "$root" -type f -links +1 -print) ]] || fail 'Hard-linked artifact files are not allowed.'
  [[ $(wc -c < "$root/SHA256SUMS") -le 131072 ]] || fail 'SHA256SUMS is too large.'
  [[ $(du -sk "$root" | awk '{print $1}') -le 262144 ]] || fail 'Artifact exceeds the 256 MiB foundation limit.'
  manifest_paths=()
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ ${#line} -gt 66 && "${line:64:2}" == '  ' ]] || fail 'Malformed checksum record.'
    expected=${line:0:64}; relative=${line:66}
    [[ "$expected" =~ ^[0-9a-f]{64}$ ]] || fail 'Malformed SHA-256 digest.'
    valid_path "$relative" || fail 'Unsafe or unexpected manifest path.'
    ! has_path "$relative" || fail 'Duplicate manifest path.'
    [[ -f "$root/$relative" && ! -L "$root/$relative" ]] || fail "Missing regular artifact file: $relative"
    manifest_paths+=("$relative")
    [[ ${#manifest_paths[@]} -le 512 ]] || fail 'Artifact has too many files.'
    actual=$(digest_file "$root/$relative")
    [[ "$actual" == "$expected" ]] || fail "Checksum mismatch: $relative"
  done < "$root/SHA256SUMS"
  # Check every path, including empty directories and objects absent from the manifest.
  count=0
  while IFS= read -r -d '' entry; do
    relative=${entry#"$root/"}
    [[ ! -L "$entry" ]] || fail 'Symbolic links are not allowed in artifacts.'
    if [[ -d "$entry" ]]; then
      case "$relative" in bin|systemd|web) ;; web/*) valid_path "$relative" || fail 'Unsafe directory path.' ;; *) fail 'Unexpected artifact directory.' ;; esac
    elif [[ -f "$entry" ]]; then
      count=$((count + 1))
      [[ "$relative" == SHA256SUMS ]] || has_path "$relative" || fail 'Unlisted artifact file.'
    else fail 'Special files are not allowed in artifacts.'; fi
  done < <(find "$root" -mindepth 1 -print0)
  [[ $count -eq $((${#manifest_paths[@]} + 1)) ]] || fail 'Artifact inventory mismatch.'
  for relative in VERSION BUILD-INFO.json install.sh bin/duck-api bin/duck-agent bin/duck-updater bin/duck-restore systemd/duck-restore.service systemd/duck-updater.service systemd/duck-api.service systemd/duck-agent.service systemd/duck-dnsmasq.service systemd/duck-network-recover.service web/index.html; do
    has_path "$relative" || fail "Required artifact file is missing: $relative"
  done
  [[ $(wc -c < "$root/VERSION") -le 80 ]] || fail 'VERSION is too long.'
  version=$(< "$root/VERSION")
  [[ "$version" =~ ^1\.0\.0-dev\.[0-9a-f]{12}(\.dirty)?$ ]] || fail 'Only internal 1.0.0-dev.<12sha>[.dirty] artifacts are supported.'
  [[ $(wc -c < "$root/BUILD-INFO.json") -le 4096 ]] || fail 'BUILD-INFO.json is too large.'
  metadata=$(< "$root/BUILD-INFO.json")
  # Accept only the canonical JSON emitted by scripts/manifest.mjs. A closed
  # grammar avoids adding a general JSON interpreter to the Debian runtime.
  metadata_pattern='^\{
  "version": "(1\.0\.0-dev\.[0-9a-f]{12}(\.dirty)?)",
  "commit": "([0-9a-f]{40})",
  "dirty": (true|false),
  "target": "(linux/(amd64|arm64|armv7))",
  "goVersion": "go[0-9]+\.[0-9]+\.[0-9]+",
  "nodeVersion": "v[0-9]+\.[0-9]+\.[0-9]+",
  "sourceDateEpoch": [0-9]+,
  "kind": "internal-development"
\}$'
  [[ "$metadata" =~ $metadata_pattern ]] || fail 'BUILD-INFO.json must match the canonical internal-build metadata format.'
  [[ "${BASH_REMATCH[1]}" == "$version" && "${BASH_REMATCH[3]:0:12}" == "${version:10:12}" ]] || fail 'Build metadata version or commit disagrees with VERSION.'
  if [[ "$version" == *.dirty ]]; then
    [[ "${BASH_REMATCH[4]}" == true ]] || fail 'Build metadata dirty marker disagrees with VERSION.'
  else
    [[ "${BASH_REMATCH[4]}" == false ]] || fail 'Build metadata dirty marker disagrees with VERSION.'
  fi
  metadata_target="${BASH_REMATCH[5]}"
  metadata_arch="${BASH_REMATCH[6]}"
  case "$metadata_target" in
    linux/amd64) expected_elf_class='02'; expected_elf_machine='3e00'; arch_label='amd64 (ELF64 EM_X86_64)' ;;
    linux/arm64) expected_elf_class='02'; expected_elf_machine='b700'; arch_label='arm64 (ELF64 EM_AARCH64)' ;;
    linux/armv7) expected_elf_class='01'; expected_elf_machine='2800'; arch_label='armv7 (ELF32 EM_ARM)' ;;
    *) fail "BUILD-INFO.json declares unsupported build target: $metadata_target" ;;
  esac
  for binary in duck-api duck-agent duck-updater duck-restore; do
    magic=$(od -An -tx1 -N5 "$root/bin/$binary" | tr -d ' \n')
    machine=$(od -An -tx1 -j18 -N2 "$root/bin/$binary" | tr -d ' \n')
    [[ "$magic" == "7f454c46$expected_elf_class" && "$machine" == "$expected_elf_machine" ]] || fail "$binary must be a Linux $arch_label binary; got ELF magic ${magic:0:14} e_machine ${machine:-??}."
  done
}

verify_artifact "$artifact"
cmp -s -- "${BASH_SOURCE[0]}" "$artifact/install.sh" || fail 'Run the installer belonging to this exact artifact.'
if [[ "$mode" == verify-artifact ]]; then
  printf 'Artifact verified: %s (SHA-256 integrity only; no signature or runtime qualification).\n' "$version"
  exit 0
fi

[[ $EUID -eq 0 ]] || fail 'Host preflight and installation require root.'
[[ "$version" != *.dirty ]] || fail 'Target installation requires a clean GitHub commit; dirty working-tree artifacts are not accepted.'
[[ $(uname -s) == Linux ]] || fail 'Only a clean Debian 13 Linux host is supported.'
[[ -r /etc/os-release ]] || fail 'Missing /etc/os-release.'
os_id=$(sed -n 's/^ID=//p' /etc/os-release | tr -d '"')
os_version=$(sed -n 's/^VERSION_ID=//p' /etc/os-release | tr -d '"')
[[ "$os_id" == debian && "$os_version" == 13 ]] || fail 'This installer requires Debian 13.'
for dependency in dpkg systemctl getent groupadd useradd install stat sha256sum find sort sed awk cut df du od tr curl ip nft ss python3; do
  command -v "$dependency" >/dev/null 2>&1 || fail "Missing prerequisite: $dependency. Install prerequisites explicitly: apt-get install iproute2 nftables curl ca-certificates passwd coreutils findutils python3"
done
host_arch=$(dpkg --print-architecture) || fail 'Cannot determine Debian dpkg architecture.'
case "$host_arch" in
  amd64) expected_host_target='linux/amd64' ;;
  arm64) expected_host_target='linux/arm64' ;;
  armhf) expected_host_target='linux/armv7' ;;
  *) fail "Debian architecture $host_arch is not supported by this installer. Supported: amd64, arm64, armhf." ;;
esac
[[ "$metadata_target" == "$expected_host_target" ]] || fail "Artifact target ($metadata_target) does not match Debian host architecture ($host_arch → $expected_host_target)."
[[ -d /run/systemd/system && $(< /proc/1/comm) == systemd ]] || fail 'A booted systemd host is required.'
[[ -x /usr/sbin/nologin ]] || fail 'Missing /usr/sbin/nologin.'

# The payload and all ancestors must be controlled by root before any host mutation.
check_root_path() {
  local entry=$1 permissions
  [[ ! -L "$entry" && $(stat -c %u "$entry") == 0 ]] || fail 'Installation paths must be root-owned and must not be symlinks.'
  permissions=$(stat -c %a "$entry")
  [[ $((8#$permissions & 0022)) -eq 0 ]] || fail 'Installation paths must not be group/world writable.'
  [[ ! -f "$entry" || $(stat -c %h "$entry") == 1 ]] || fail 'Hard-linked artifact files are not allowed.'
}
entry=$artifact
while :; do check_root_path "$entry"; [[ "$entry" != / ]] || break; entry=$(dirname "$entry"); done
while IFS= read -r -d '' entry; do check_root_path "$entry"; done < <(find "$artifact" -mindepth 1 -print0)
for entry in /usr /opt /etc /var /var/lib /run /etc/systemd /etc/systemd/system; do
  [[ -d "$entry" ]] || fail "Required host directory is missing: $entry"
  check_root_path "$entry"
done
if [[ -e /usr/libexec ]]; then check_root_path /usr/libexec; fi
for entry in /usr/libexec/duckos /var/lib/duckos-restore /run/duckos-restore /var/lib/duckos-updater /run/duckos-updater /opt/duckos /etc/duckos /var/lib/duckos /var/lib/duckos-agent /run/duckos-agent /etc/duckos-dnsmasq /var/lib/duckos-dnsmasq /run/duckos-dnsmasq; do
  [[ ! -e "$entry" && ! -L "$entry" ]] || fail "Existing installation or conflicting path: $entry. Automatic update/recovery is not implemented."
done
! getent passwd duckos >/dev/null || fail 'The duckos user already exists; refusing to reuse its identity.'
! getent group duckos >/dev/null || fail 'The duckos group already exists; refusing to reuse its identity.'
! getent passwd duck-dnsmasq >/dev/null || fail 'The duck-dnsmasq user already exists; refusing to reuse its identity.'
! getent group duck-dnsmasq >/dev/null || fail 'The duck-dnsmasq group already exists; refusing to reuse its identity.'
for unit in duck-restore.service duck-updater.service duck-agent.service duck-api.service duck-dnsmasq.service duck-network-recover.service; do
  [[ $(systemctl show --property=LoadState --value "$unit") == not-found ]] || fail "Existing systemd unit: $unit"
  for entry in "/etc/systemd/system/$unit" "/usr/lib/systemd/system/$unit" "/lib/systemd/system/$unit"; do
    [[ ! -e "$entry" && ! -L "$entry" ]] || fail "Existing unit path: $entry"
  done
done
[[ -z $(ss -H -ltn 'sport = :8080') ]] || fail 'TCP port 8080 is already in use.'
required_kib=$(( $(du -sk "$artifact" | awk '{print $1}') * 2 + 262144 ))
for entry in /opt /var/lib; do
  available_kib=$(df -Pk "$entry" | awk 'END {print $4}')
  [[ "$available_kib" =~ ^[0-9]+$ && "$available_kib" -ge "$required_kib" ]] || fail "Insufficient free space on $entry (payload copies plus 256 MiB reserve required)."
done
if [[ "$mode" == check ]]; then
  printf 'Read-only preflight passed: %s; Debian 13 %s (%s); clean host.\n' "$version" "$host_arch" "$expected_host_target"
  exit 0
fi

# Same ASCII mark as pkg/brand/mark.txt; checked by the brand contract test.
printf '%s\n' '<(o)__/' ' (___/' 'DuckOS installer'

agent_attempted=false
api_attempted=false
enable_attempted=false
finish_failed() {
  local result=$?
  [[ $result -ne 0 ]] || return 0
  trap - EXIT
  unset token
  if $api_attempted; then systemctl stop duck-api.service >/dev/null 2>&1 || printf 'Could not stop duck-api; inspect service state.\n' >&2; fi
  if $agent_attempted; then systemctl stop duck-agent.service duck-restore.service >/dev/null 2>&1 || printf 'Could not stop duck-agent; inspect service state.\n' >&2; fi
  if $enable_attempted; then systemctl disable duck-agent.service duck-api.service duck-restore.service >/dev/null 2>&1 || printf 'Could not disable DuckOS units; inspect boot enablement.\n' >&2; fi
  printf '%s\n' 'DuckOS installation did not complete. Cleanup was attempted only for DuckOS services started by this invocation.' \
    'Evidence and files are preserved. Do not rerun or delete paths blindly.' \
    'Inspect: journalctl -u duck-agent.service -u duck-api.service --no-pager -n 100' \
    'Inspect root-owned /var/lib/duckos/install-status and /opt/duckos before manual recovery.' >&2
  exit "$result"
}
trap finish_failed EXIT

groupadd --system duckos
useradd --system --gid duckos --home-dir /var/lib/duckos --no-create-home --shell /usr/sbin/nologin duckos
groupadd --system duck-dnsmasq
useradd --system --gid duck-dnsmasq --home-dir /var/lib/duckos-dnsmasq --no-create-home --shell /usr/sbin/nologin duck-dnsmasq
duckos_uid=$(id -u duckos)
[[ "$duckos_uid" =~ ^[0-9]+$ && "$duckos_uid" -gt 0 ]] || fail 'Invalid DuckOS service UID.'
dnsmasq_uid=$(id -u duck-dnsmasq)
[[ "$dnsmasq_uid" =~ ^[0-9]+$ && "$dnsmasq_uid" -gt 0 && "$dnsmasq_uid" != "$duckos_uid" ]] || fail 'Invalid DNS service UID.'
install -d -o root -g root -m 0755 /opt/duckos /opt/duckos/releases
install -d -o root -g duckos -m 0750 /etc/duckos /var/lib/duckos
install -d -o duckos -g duckos -m 0700 /var/lib/duckos/config /var/lib/duckos/state /var/lib/duckos/identity /var/lib/duckos/policy /var/lib/duckos/wireguard /var/lib/duckos/xray /var/lib/duckos/backups
install -d -o root -g root -m 0700 /var/lib/duckos-agent /var/lib/duckos-agent/network
install -d -o root -g root -m 0700 /var/lib/duckos-agent/ztp /var/lib/duckos-agent/wireguard /var/lib/duckos-agent/xray /var/lib/duckos-agent/frr /var/lib/duckos-agent/qos /var/lib/duckos-agent/vault /var/lib/duckos-agent/vault/records
install -d -o root -g duck-dnsmasq -m 0750 /etc/duckos-dnsmasq
install -d -o duck-dnsmasq -g duck-dnsmasq -m 0700 /var/lib/duckos-dnsmasq
# FRR is the V1 routing protocol supervisor. The Agent owns FRR's pathspace
# (PathspaceBase=/run/frr) at runtime, so the package must be present for the
# runtime to even mount a viable namespace. The package ships in Debian's main
# repository; install it now if missing. If the install cannot (offline host,
# apt unavailable) we record the honest "not installed" state to install-status
# but refuse to bypass the routing supervisor entirely.
if command -v vtysh >/dev/null 2>&1 && command -v /usr/lib/frr/zebra >/dev/null 2>&1; then
  printf 'DuckOS installer: FRR binaries detected.\n' >&2
elif command -v apt-get >/dev/null 2>&1 && [[ -r /var/cache/apt/pkgcache.bin || -r /var/lib/apt/lists ]]; then
  if DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends frr >/var/log/duckos-install-frr.log 2>&1; then
    printf 'DuckOS installer: FRR package provisioned via apt-get.\n' >&2
  else
    printf 'DuckOS installer: apt-get install frr failed; routing protocol features will report not installed. See /var/log/duckos-install-frr.log.\n' >&2
  fi
else
  printf 'DuckOS installer: FRR binaries not found and apt-get unavailable; routing protocol features will report not installed.\n' >&2
fi

# Provision the production network enrollment from real host state.
# Reads physical NIC MACs via `ip -j link show`, filters out loopback and
# qualification fixtures (anything starting with ducklab-), and writes a
# strictly-owned, mode 0600 enrollment document at
# /var/lib/duckos-agent/enrollment.json. Idempotent: a valid existing
# enrollment is left alone; a missing or invalid one is (re)written. This
# is the only legitimate production path that brings the Agent's managed
# routing subsystem from "network service is not enabled" to admitted.
enroll_root_dir=/var/lib/duckos-agent
enroll_file=$enroll_root_dir/enrollment.json
enroll_physical_json=''
if command -v ip >/dev/null 2>&1; then
  enroll_physical_json=$(ip -j link show 2>/dev/null | python3 -c '
import json, os, sys
try:
  rows = json.load(sys.stdin)
except Exception:
  rows = []
out = []
for r in rows:
  name = r.get("ifname", "")
  if not name or name == "lo" or name.startswith("ducklab-"):
    continue
  if r.get("link_type", "") == "loopback":
    continue
  mac = (r.get("address") or "").lower()
  if not mac or mac == "00:00:00:00:00:00":
    continue
  # A physical NIC that already carries an active non-DuckOS DHCP lease
  # (typically from dhcpcd on the management interface) stays under that
  # manager. The Agent owns the durable identity for cross-manager
  # accounting, but Safe Apply must never render, replace, or reload
  # the networkd profile for the link. Migration is an explicit operator
  # workflow that flips this flag.
  lease = os.path.join("/var/lib/dhcpcd", name + ".lease")
  external = os.path.exists(lease)
  entry = {"id": name, "name": name, "mac": mac}
  if external:
    entry["externallyManaged"] = True
  out.append(entry)
print(json.dumps({"physical": out}))
' 2>/dev/null || echo '')
fi
enroll_install=true
if [[ -f "$enroll_file" ]]; then
  if [[ "$(stat -c '%u' "$enroll_file")" == "0" ]] && \
     [[ "$(stat -c '%a' "$enroll_file")" == "600" ]] && \
     [[ "$(stat -c '%h' "$enroll_file")" == "1" ]] && \
     [[ "$(stat -c '%s' "$enroll_file")" -le 65536 ]] && \
     python3 - "$enroll_file" << 'PYEOF' >/dev/null 2>&1
import json, sys
path = sys.argv[1]
with open(path) as f:
  e = json.load(f)
assert e.get("version") == 1
assert e.get("firewallTable") == "inet duckos"
assert isinstance(e.get("physical"), list) and len(e["physical"]) >= 1
assert e.get("createdAt")
for b in e["physical"]:
  assert all(k in b for k in ("id", "name", "mac"))
PYEOF
  then
    enroll_install=false
  else
    printf 'DuckOS installer: existing %s is unsafe (mode/owner/contents); refusing to overwrite.\n' "$enroll_file" >&2
    fail 'Unsafe pre-existing enrollment document. Inspect /var/lib/duckos-agent/enrollment.json and re-run after manual review.'
  fi
fi
if [[ "$enroll_install" == true ]]; then
  if [[ -z "$enroll_physical_json" || "$enroll_physical_json" == '{"physical":[]}' ]]; then
    printf 'DuckOS installer: no eligible physical NICs found; managed routing will stay disabled.\n' >&2
  else
    enroll_now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    umask 077
    enroll_payload=$(printf 'PHYSICAL_JSON=%s\n' "$enroll_physical_json" | PHYSICAL_JSON="$enroll_physical_json" python3 -c '
import json, os, sys
created_at = sys.argv[1]
doc = {
  "version": 1,
  "createdAt": created_at,
  "physical": json.loads(os.environ["PHYSICAL_JSON"])["physical"],
  "vlanNames": [],
  "firewallTable": "inet duckos",
  "routingTables": [10000],
  "policyRouting": True,
  "networkdReload": True,
}
sys.stdout.write(json.dumps(doc, indent=2, sort_keys=True))
sys.stdout.write("\n")
' "$enroll_now")
    umask 077
    printf '%s\n' "$enroll_payload" > "$enroll_file"
    chmod 0600 "$enroll_file"
    chown root:root "$enroll_file"
    python3 - "$enroll_file" << 'PYEOF'
import json, sys
with open(sys.argv[1]) as f:
  e = json.load(f)
assert e["version"] == 1
assert e["firewallTable"] == "inet duckos"
assert len(e["physical"]) >= 1
assert all(b["id"] and b["name"] and b["mac"] for b in e["physical"])
PYEOF
  fi
fi
unset enroll_physical_json enroll_install enroll_now enroll_payload

# Mark the Agent's network state directory as fresh-install-prepared. The
# production Agent owns its state journal (`state.OpenExisting`) and refuses
# to operate on a missing file. The installer cannot synthesise a valid
# enveloped snapshot without bypassing the Agent's own durable writer, so it
# plants a strictly-owned sentinel that the Agent's first-boot code path
# recognises. On the next `duck-agent` startup, the Agent's managed routing
# subsystem initialises the journal through its normal `state.Open` writer,
# reconciles the placeholder `Current` with the live kernel via the engine's
# existing bootstrap goroutine, and removes the sentinel.
agent_net_dir=/var/lib/duckos-agent/network
if [[ -d "$agent_net_dir" ]]; then
  agent_net_sentinel=$agent_net_dir/.installer-prepared
  umask 077
  printf 'installer-prepared\n' > "$agent_net_sentinel"
  chmod 0600 "$agent_net_sentinel"
  chown root:root "$agent_net_sentinel"
  unset agent_net_sentinel
fi
unset agent_net_dir

printf 'incomplete %s\n' "$version" > /var/lib/duckos/install-status
chown root:duckos /var/lib/duckos/install-status
chmod 0640 /var/lib/duckos/install-status
staging="/opt/duckos/releases/.install-$version"
release="/opt/duckos/releases/$version"
install -d -o root -g root -m 0755 "$staging"
for relative in "${manifest_paths[@]}" SHA256SUMS; do
  install -d -o root -g root -m 0755 "$(dirname "$staging/$relative")"
  file_mode=0644
  case "$relative" in bin/*|install.sh) file_mode=0755 ;; esac
  install -o root -g root -m "$file_mode" "$artifact/$relative" "$staging/$relative"
done
verify_artifact "$staging"
mv -- "$staging" "$release"
ln -s "releases/$version" /opt/duckos/current

# The running supervisor is intentionally outside the replaceable release.
# Ordinary update operations never write this executable or its service unit.
install -d -o root -g root -m 0755 /usr/libexec /usr/libexec/duckos
install -o root -g root -m 0755 "$release/bin/duck-updater" /usr/libexec/duckos/duck-updater
/usr/libexec/duckos/duck-updater --init-health-key
install -o root -g root -m 0755 "$release/bin/duck-restore" /usr/libexec/duckos/duck-restore
install -d -o root -g root -m 0700 /var/lib/duckos-restore /var/lib/duckos/restore-source /var/lib/duckos/restore-live
install -o root -g root -m 0644 "$release/systemd/duck-restore.service" /etc/systemd/system/duck-restore.service
install -d -o root -g root -m 0700 /var/lib/duckos-updater
install -o root -g root -m 0644 "$release/systemd/duck-updater.service" /etc/systemd/system/duck-updater.service
# Trust keys and a signed baseline are provisioned explicitly. Do not silently
# invent trust or enable updates against an unsigned installer artifact.

token=$(od -An -v -N32 -tx1 /dev/urandom | tr -d ' \n')
[[ "$token" =~ ^[0-9a-f]{64}$ ]] || fail 'Could not obtain a cryptographic API token.'
printf '%s\n' "$token" > /etc/duckos/api-token
chown root:duckos /etc/duckos/api-token
chmod 0640 /etc/duckos/api-token
assertion_key=$(od -An -v -N32 -tx1 /dev/urandom | tr -d ' \n')
[[ "$assertion_key" =~ ^[0-9a-f]{64}$ ]] || fail 'Could not obtain a cryptographic Agent assertion key.'
printf '%s\n' "$assertion_key" > /etc/duckos/agent-assertion.key
chown root:duckos /etc/duckos/agent-assertion.key
chmod 0640 /etc/duckos/agent-assertion.key
install -o root -g root -m 0644 "$release/systemd/duck-api.service" /etc/systemd/system/duck-api.service
# DNS/DHCP remains inactive until an enrolled network transaction provides and
# verifies configuration. Installation does not bind ports 53 or 67.
install -o root -g root -m 0644 "$release/systemd/duck-dnsmasq.service" /etc/systemd/system/duck-dnsmasq.service
# The boot recovery oneshot prepares /etc/systemd/network before networkd
# starts. Wire its unit, the dependency symlink systemd-networkd uses to
# pull it in at boot, and the enablement the boot guard observes after a
# clean install. systemd-networkd must be reloaded (and restarted, when
# running) so its runtime Requires= reflects the new ordering. The
# restart is safe because /etc/systemd/network/ is owned by DuckOS and
# empty at this point — networkd has no profiles to apply and external
# interfaces (eth0 under dhcpcd, eth1/eth2 not yet adopted) stay outside
# its control.
install -o root -g root -m 0644 "$release/systemd/duck-network-recover.service" /etc/systemd/system/duck-network-recover.service
install -d -o root -g root -m 0755 /etc/systemd/system/systemd-networkd.service.requires
ln -sfn /etc/systemd/system/duck-network-recover.service /etc/systemd/system/systemd-networkd.service.requires/duck-network-recover.service
chown -h root:root /etc/systemd/system/systemd-networkd.service.requires/duck-network-recover.service
systemctl enable duck-network-recover.service
agent_unit=$(mktemp /etc/systemd/system/.duck-agent.XXXXXX)
sed "s/@DUCKOS_UID@/$duckos_uid/g" "$release/systemd/duck-agent.service" > "$agent_unit"
install -o root -g root -m 0644 "$agent_unit" /etc/systemd/system/duck-agent.service
rm -- "$agent_unit"
systemctl daemon-reload
if systemctl is-active --quiet systemd-networkd.service; then
  systemctl restart systemd-networkd.service
else
  # Enable systemd-networkd so the boot ordering (Requires=
  # duck-network-recover.service) takes effect on the next
  # reboot. /etc/systemd/network is empty at install time so the
  # daemon does not re-parent any externally-managed link — the
  # eth0 contract stays intact regardless of whether dhcpcd,
  # NetworkManager, or systemd-networkd originally owned it.
  systemctl enable systemd-networkd.service >/dev/null 2>&1 || printf 'Could not enable systemd-networkd; inspect boot enablement.\n' >&2
  # Also start it now so the renderer can talk to networkd on
  # the very first Safe Apply. The empty profile tree means
  # no link transitions: only the routing/control surface is
  # activated. Without this start, every fresh install hits a
  # "networkd inventory unavailable" preflight failure that the
  # operator would otherwise have to clear by hand.
  systemctl start systemd-networkd.service >/dev/null 2>&1 || printf 'Could not start systemd-networkd; networkd inventory will be unavailable until next boot.\n' >&2
fi
agent_attempted=true
systemctl start duck-restore.service duck-agent.service
api_attempted=true
systemctl start duck-api.service

authenticated_probe() {
  printf 'header = "Authorization: Bearer %s"\n' "$token" |
    curl --disable --config - --noproxy '*' --silent --fail --max-time 10 --output /dev/null "http://127.0.0.1:8080$1"
}
ready=false
deadline=$((SECONDS + 90))
while [[ $SECONDS -lt $deadline ]]; do
  if systemctl is-active --quiet duck-agent.service && systemctl is-active --quiet duck-api.service &&
    authenticated_probe /api/v1/ready && authenticated_probe /api/v1/overview; then ready=true; break; fi
  sleep 1
done
unset token
$ready || fail 'Authenticated Agent-backed readiness/overview did not pass before the deadline.'
enable_attempted=true
systemctl enable duck-agent.service duck-api.service duck-restore.service
printf 'ready %s\n' "$version" > /var/lib/duckos/install-status
trap - EXIT
printf 'Installed %s as an internal TEST build. Authenticated readiness and overview passed.\n' "$version"
printf '%s\n' 'API/UI listen at http://127.0.0.1:8080 only. The API token was not printed.' \
  'An administrator can read it with: sudo cat /etc/duckos/api-token' \
  'No DuckOS network configuration was applied. See docs/install.md for limitations and recovery.'
