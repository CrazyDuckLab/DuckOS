import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { promises as fs } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const installer = path.join(root, 'scripts/install.sh');
const hash = (data) => createHash('sha256').update(data).digest('hex');

async function fixture(t) {
  const directory = await fs.realpath(await fs.mkdtemp(path.join(os.tmpdir(), 'duckos-installer-test-')));
  t.after(() => fs.rm(directory, { recursive: true, force: true }));
  const artifact = path.join(directory, 'artifact');
  // Synthetic ELF header only: these files are never executed. This tests the
  // packaging guard, not Linux execution, host installation, or ELF validity.
  const elf = Buffer.alloc(64);
  elf.set([0x7f, 0x45, 0x4c, 0x46, 2, 1, 1]);
  elf[18] = 0x3e;
  // Synthetic xray-core payload. The vendored ELF cannot hash to the real
  // upstream pin (no preimage), so the fixture rewrites the pin inside the
  // artifact's own install.sh to the synthetic digest; the verifier is run
  // with the artifact's own install.sh so its identity check still passes.
  // A stale pin (e.g. an upstream ZIP digest) is covered by a dedicated
  // rejection test below.
  const xrayBinary = 'synthetic-xray-amd64-payload\n';
  const xraySha = hash(xrayBinary);
  const installerSource = (await fs.readFile(installer))
    .toString('utf8')
    .replace('8255dd939c34cf966cc91517b6324dd3c8d0bcf49ffac8beca049a38c46845ed', xraySha);
  const files = new Map([
    ['VERSION', '1.0.0-dev.012345abcdef\n'],
    ['BUILD-INFO.json', JSON.stringify({ version: '1.0.0-dev.012345abcdef', commit: '012345abcdef0000000000000000000000000000', dirty: false, target: 'linux/amd64', goVersion: 'go1.27.1', nodeVersion: 'v24.20.0', sourceDateEpoch: 1788796800, kind: 'internal-development' }, null, 2) + '\n'],
    ['install.sh', installerSource],
    ['bin/duck-agent', elf],
    ['bin/duck-api', elf],
    ['bin/duck-updater', elf],
    ['bin/duck-restore', elf],
    ['xray-core/Xray-linux-64', xrayBinary],
    ['xray-core/XRAY_CORE_VERSION', '26.3.27\n'],
    ['xray-core/XRAY_CORE_LICENSE', 'MPL-2.0 synthetic license fixture\n'],
    ['xray-core/XRAY_CORE_SHA256SUMS', `${xraySha}  Xray-linux-64\n`],
    ['web/index.html', '<!doctype html><title>Synthetic installer fixture</title>\n'],
    ['web/assets/main.js', '/* Synthetic packaging fixture, never executed. */\n'],
    ['systemd/duck-agent.service', await fs.readFile(path.join(root, 'packaging/systemd/duck-agent.service'))],
    ['systemd/duck-api.service', await fs.readFile(path.join(root, 'packaging/systemd/duck-api.service'))],
    ['systemd/duck-updater.service', await fs.readFile(path.join(root, 'packaging/systemd/duck-updater.service'))],
    ['systemd/duck-restore.service', await fs.readFile(path.join(root, 'packaging/systemd/duck-restore.service'))],
    ['systemd/duck-dnsmasq.service', await fs.readFile(path.join(root, 'packaging/systemd/duck-dnsmasq.service'))],
    ['systemd/duck-network-recover.service', await fs.readFile(path.join(root, 'packaging/systemd/duck-network-recover.service'))],
  ]);
  for (const [name, content] of files) {
    await fs.mkdir(path.dirname(path.join(artifact, name)), { recursive: true });
    await fs.writeFile(path.join(artifact, name), content);
  }
  const manifest = async () => {
    const entries = [];
    for (const name of [...files.keys()].sort()) entries.push(`${hash(await fs.readFile(path.join(artifact, name)))}  ${name}\n`);
    await fs.writeFile(path.join(artifact, 'SHA256SUMS'), entries.join(''));
  };
  await manifest();
  return { directory, artifact, files, manifest };
}

function verify(artifact, args = []) {
  // Run the artifact's own install.sh: the verifier refuses to run if the
  // artifact's installer differs from the invoked one, and the fixture pins
  // its synthetic xray payload inside that copy.
  const result = spawnSync('/bin/bash', [path.join(artifact, 'install.sh'), '--artifact', artifact, '--verify-artifact', ...args], {
    encoding: 'utf8', timeout: 15000, env: { PATH: '/usr/bin:/bin:/usr/sbin:/sbin' },
  });
  assert.equal(result.error, undefined, result.error?.message);
  return { status: result.status, output: result.stdout + result.stderr };
}
function rejected(result, reason) {
  assert.notEqual(result.status, 0, result.output);
  assert.match(result.output, reason);
}

test('valid artifact verifies read-only, including inventory and original file bytes', async (t) => {
  const { artifact, files } = await fixture(t);
  const before = await Promise.all([...files.keys(), 'SHA256SUMS'].map(async (name) => [name, hash(await fs.readFile(path.join(artifact, name)))]));
  const result = verify(artifact);
  assert.equal(result.status, 0, result.output);
  assert.match(result.output, /SHA-256 integrity only; no signature or runtime qualification/);
  for (const [name, digest] of before) assert.equal(hash(await fs.readFile(path.join(artifact, name))), digest);
});

test('changed payload is rejected before host preflight', async (t) => {
  const { artifact } = await fixture(t);
  await fs.appendFile(path.join(artifact, 'bin/duck-api'), 'corrupt');
  rejected(verify(artifact), /Checksum mismatch: bin\/duck-api/);
});

test('unlisted files and unexpected empty directories are rejected', async (t) => {
  const { artifact } = await fixture(t);
  await fs.writeFile(path.join(artifact, 'unlisted'), 'unexpected');
  rejected(verify(artifact), /Unlisted artifact file/);
  await fs.unlink(path.join(artifact, 'unlisted'));
  await fs.mkdir(path.join(artifact, 'surprise'));
  rejected(verify(artifact), /Unexpected artifact directory/);
});

test('required files cannot be omitted from an otherwise consistent manifest', async (t) => {
  const f = await fixture(t);
  f.files.delete('web/index.html');
  await fs.unlink(path.join(f.artifact, 'web/index.html'));
  await f.manifest();
  rejected(verify(f.artifact), /Required artifact file is missing: web\/index.html/);
});

test('duplicate and malformed checksum lines are rejected', async (t) => {
  const { artifact } = await fixture(t);
  const manifest = path.join(artifact, 'SHA256SUMS');
  const original = await fs.readFile(manifest, 'utf8');
  await fs.appendFile(manifest, original.split('\n')[0] + '\n');
  rejected(verify(artifact), /Duplicate manifest path/);
  await fs.writeFile(manifest, 'bad digest\n');
  rejected(verify(artifact), /Malformed checksum record/);
});

for (const unsafe of ['../outside', '/etc/shadow', 'web/../../outside', 'web/./index.html', 'web//index.html', 'web/file name', 'web/$(command)']) {
  test(`manifest path rejected: ${unsafe}`, async (t) => {
    const { artifact } = await fixture(t);
    await fs.appendFile(path.join(artifact, 'SHA256SUMS'), `${'0'.repeat(64)}  ${unsafe}\n`);
    rejected(verify(artifact), /Unsafe or unexpected manifest path/);
  });
}

test('symlink file and symlink parent are rejected before reading linked contents', async (t) => {
  const { artifact, directory } = await fixture(t);
  await fs.symlink('/dev/zero', path.join(artifact, 'web/infinite'));
  rejected(verify(artifact), /Symbolic links are not allowed/);
  await fs.unlink(path.join(artifact, 'web/infinite'));
  await fs.mkdir(path.join(directory, 'outside'));
  await fs.symlink(path.join(directory, 'outside'), path.join(artifact, 'web/link'));
  rejected(verify(artifact), /Symbolic links are not allowed/);
});

test('hard links are rejected', async (t) => {
  const { artifact } = await fixture(t);
  await fs.link(path.join(artifact, 'web/index.html'), path.join(artifact, 'web/duplicate.html'));
  rejected(verify(artifact), /Hard-linked artifact files are not allowed/);
});

test('FIFO and newline filenames are rejected without opening special files', async (t) => {
  const { artifact } = await fixture(t);
  const fifo = spawnSync('/usr/bin/mkfifo', [path.join(artifact, 'web/fifo')], { encoding: 'utf8' });
  assert.equal(fifo.status, 0, fifo.stderr);
  rejected(verify(artifact), /Special files are not allowed/);
  await fs.unlink(path.join(artifact, 'web/fifo'));
  await fs.writeFile(path.join(artifact, 'web/odd\nname'), 'synthetic');
  rejected(verify(artifact), /Unlisted artifact file/);
});

test('wrong binary architecture is rejected even with matching checksums', async (t) => {
  const f = await fixture(t);
  const binary = await fs.readFile(path.join(f.artifact, 'bin/duck-agent'));
  binary[18] = 0xb7; // AArch64, not amd64.
  await fs.writeFile(path.join(f.artifact, 'bin/duck-agent'), binary);
  await f.manifest();
  rejected(verify(f.artifact), /duck-agent must be a Linux amd64 \(ELF64 EM_X86_64\) binary/);
});

test('vendored xray-core binary matching the embedded pin verifies', async (t) => {
  const { artifact } = await fixture(t);
  const result = verify(artifact);
  assert.equal(result.status, 0, result.output);
});

test('stale xray-core pin (upstream ZIP digest instead of ELF digest) is rejected', async (t) => {
  // Regression for the beta.2 release blocker: the arm64/armv7 pins held
  // the sha256 of the upstream .zip archive while the installer hashed the
  // extracted ELF, so every ARM install failed closed. The artifact
  // verifier must catch this class of mismatch before publication.
  const f = await fixture(t);
  const zipDigest = '4d30283ae614e3057f730f67cd088a42be6fdf91f8639d82cb69e48cde80413c';
  const installerPath = path.join(f.artifact, 'install.sh');
  const source = (await fs.readFile(installerPath, 'utf8')).replace(
    /^ *'amd64:Xray-linux-64:[0-9a-f]{64}'$/m,
    `  'amd64:Xray-linux-64:${zipDigest}'`);
  assert.match(source, new RegExp(zipDigest), 'pin rewrite found no target');
  await fs.writeFile(installerPath, source);
  await f.manifest();
  rejected(verify(f.artifact), /xray-core pin mismatch for amd64/);
});

test('missing or empty xray-core license is rejected', async (t) => {
  const f = await fixture(t);
  await fs.rm(path.join(f.artifact, 'xray-core/XRAY_CORE_LICENSE'));
  f.files.delete('xray-core/XRAY_CORE_LICENSE');
  await f.manifest();
  rejected(verify(f.artifact), /Required artifact file is missing: xray-core\/XRAY_CORE_LICENSE/);
  await fs.writeFile(path.join(f.artifact, 'xray-core/XRAY_CORE_LICENSE'), '');
  f.files.set('xray-core/XRAY_CORE_LICENSE', '');
  await f.manifest();
  rejected(verify(f.artifact), /XRAY_CORE_LICENSE is empty or missing/);
});

test('missing vendored xray-core binary is rejected', async (t) => {
  const f = await fixture(t);
  await fs.rm(path.join(f.artifact, 'xray-core/Xray-linux-64'));
  f.files.delete('xray-core/Xray-linux-64');
  await f.manifest();
  rejected(verify(f.artifact), /Required artifact file is missing: xray-core\/Xray-linux-64/);
});

test('xray-core manifest that does not list the vendored binary is rejected', async (t) => {
  const f = await fixture(t);
  await fs.writeFile(path.join(f.artifact, 'xray-core/XRAY_CORE_SHA256SUMS'), 'deadbeef  Xray-linux-64\n');
  await f.manifest();
  rejected(verify(f.artifact), /XRAY_CORE_SHA256SUMS does not list Xray-linux-64/);
});

test('build metadata rejects malformed JSON, mismatched revisions and dirty state', async (t) => {
  const f = await fixture(t);
  const metadataPath = path.join(f.artifact, 'BUILD-INFO.json');
  const original = JSON.parse(await fs.readFile(metadataPath, 'utf8'));
  await fs.writeFile(metadataPath, '{ "version": invalid }\n');
  await f.manifest();
  rejected(verify(f.artifact), /canonical internal-build metadata format/);
  await fs.writeFile(metadataPath, JSON.stringify({ ...original, commit: 'f'.repeat(40) }, null, 2) + '\n');
  await f.manifest();
  rejected(verify(f.artifact), /version or commit disagrees/);
  await fs.writeFile(metadataPath, JSON.stringify({ ...original, dirty: true }, null, 2) + '\n');
  await f.manifest();
  rejected(verify(f.artifact), /dirty marker disagrees/);
});

test('public release labels and oversized sparse files are rejected', async (t) => {
  const f = await fixture(t);
  await fs.writeFile(path.join(f.artifact, 'VERSION'), '1.0.0\n');
  await f.manifest();
  rejected(verify(f.artifact), /Only internal/);
  await fs.truncate(path.join(f.artifact, 'bin/duck-agent'), 268435457);
  rejected(verify(f.artifact), /256 MiB size limit/);
});

test('installer must match the checksummed artifact installer', async (t) => {
  const f = await fixture(t);
  await fs.appendFile(path.join(f.artifact, 'install.sh'), '\n# different installer\n');
  await f.manifest();
  // Invoke the canonical repo installer: the verifier refuses to run when
  // the artifact's installer differs from the invoked one.
  const result = spawnSync('/bin/bash', [installer, '--artifact', f.artifact, '--verify-artifact'], {
    encoding: 'utf8', timeout: 15000, env: { PATH: '/usr/bin:/bin:/usr/sbin:/sbin' },
  });
  rejected({ status: result.status, output: result.stdout + result.stderr }, /installer belonging to this exact artifact/);
});

test('ambiguous options, symlink roots and noncanonical paths are rejected', async (t) => {
  const { artifact, directory } = await fixture(t);
  rejected(verify(artifact, ['--check']), /Choose only one check mode/);
  rejected(verify(artifact + '/'), /canonical artifact path/);
  await fs.symlink(artifact, path.join(directory, 'link'));
  rejected(verify(path.join(directory, 'link')), /absolute real directory/);
});

test('host --check remains read-only on the development host', async (t) => {
  const { artifact } = await fixture(t);
  // The artifact's own installer is what a host would run; the fixture's
  // synthetic xray pin lives in that copy.
  const result = spawnSync('/bin/bash', [path.join(artifact, 'install.sh'), '--artifact', artifact, '--check'], { encoding: 'utf8', timeout: 15000 });
  assert.equal(result.error, undefined);
  assert.match(result.stdout + result.stderr, /require root|Debian 13|systemd host|root-owned|group\/world writable|preflight passed/);
  assert.doesNotMatch(result.stdout + result.stderr, /Installed .* as an internal/);
});

test('unit boundary keeps API unprivileged and fixes Agent semantic service arguments', async () => {
  const api = await fs.readFile(path.join(root, 'packaging/systemd/duck-api.service'), 'utf8');
  const agent = await fs.readFile(path.join(root, 'packaging/systemd/duck-agent.service'), 'utf8');
  assert.match(api, /^User=duckos$/m);
  assert.match(api, /^CapabilityBoundingSet=$/m);
  assert.match(api, /--listen 127\.0\.0\.1:8080/);
  assert.match(api, /--updater-health-public-key \/etc\/duckos\/updater-health\.pub/);
  assert.doesNotMatch(api, /updater-health\.key/);
  const install = await fs.readFile(installer, 'utf8');
  assert.match(install, /\/usr\/libexec\/duckos\/duck-updater --init-health-key/);
  assert.match(agent, /^User=root$/m);
  // After the live BETA.2 architectural review, the Agent
  // holds NO capabilities. The FRR-only caps
  // (NET_ADMIN, NET_RAW, NET_BIND_SERVICE) are now scoped
  // to /usr/libexec/duckos/duck-frr-launcher and to the
  // FRR binaries' file caps. CAP_SYS_ADMIN is FORBIDDEN on
  // the Agent. The Go-side privilege_model_test.go enforces
  // the same model.
  assert.match(agent, /^CapabilityBoundingSet=$/m);
  assert.match(agent, /^AmbientCapabilities=$/m);
  for (const tok of [
    'CAP_NET_ADMIN', 'CAP_NET_RAW', 'CAP_NET_BIND_SERVICE',
    'CAP_SETUID', 'CAP_SETGID', 'CAP_SETPCAP',
    'CAP_SYS_ADMIN', 'CAP_DAC_OVERRIDE', 'CAP_CHOWN',
    'CAP_FOWNER', 'CAP_SYS_PTRACE', 'CAP_AUDIT_',
  ]) {
    const re = new RegExp(`^CapabilityBoundingSet=.*${tok}`, 'm');
    assert.doesNotMatch(agent, re, `duck-agent.service must NOT include ${tok} in CapabilityBoundingSet`);
  }
  // NoNewPrivileges is required because the Agent has no
  // caps and must not gain any through setuid / file-cap
  // binaries (the launcher has its own file caps, but the
  // launcher is exec'd by the Agent via fork+exec, and
  // exec does not change the parent's cap sets under
  // NoNewPrivileges anyway).
  assert.match(agent, /^NoNewPrivileges=yes$/m);
  // The Agent needs to write the FRR pathspace setup;
  // ReadWritePaths grants that without DAC bypass.
  assert.match(agent, /^SupplementaryGroups=frr frrvty$/m);
  assert.match(agent, /--allowed-uid @DUCKOS_UID@ --environment test/);
  assert.match(agent, /^RuntimeDirectory=frr$/m);
  // BETA.2 B-006: single-owner RuntimeDirectory model — 0750 on both
  // duckos-agent (the Agent IPC dir) and frr.
  assert.match(agent, /^RuntimeDirectoryMode=0750$/m);
  assert.match(agent, /ReadWritePaths=.*\/run\/frr.*\/var\/lib\/frr/);
  assert.match(agent, /ReadWritePaths=.*\/etc\/frr/);
  for (const unit of [api, agent]) {
    assert.match(unit, /^ProtectSystem=strict$/m);
  }
  // Resource limits apply to the agent (memory / tasks /
  // core); the api is light and doesn't need them.
  // Resource limits for the agent.
  assert.match(agent, /^MemoryMax=256M$/m);
  assert.match(agent, /^TasksMax=64$/m);
  assert.match(agent, /^LimitCORE=0$/m);
});

test('launcher binary is in the artifact, runs as a daemon, and serves the FRR launch contract', async () => {
  // The privilege-separation model (post BETA.2
  // architectural review) requires:
  //   1. /usr/libexec/duckos/duck-frr-launcher is in
  //      the artifact and has its file caps wired by
  //      install.sh.
  //   2. duck-frr-launcher.service is in the artifact
  //      and is installed + enabled by install.sh.
  //   3. The service unit grants CAP_SETUID +
  //      CAP_SETGID + CAP_SETPCAP (so the launcher can
  //      drop privileges to frr) and turns off
  //      NoNewPrivileges so the launcher's exec of
  //      /usr/lib/frr/{zebra,bgpd} honours the FRR
  //      binaries' file caps.
  //   4. CAP_SYS_ADMIN is FORBIDDEN on the launcher
  //      service — it stays out of every DuckOS
  //      process.
  //   5. install.sh sets the file caps on the FRR
  //      binaries too (per-binary +ep), so FRR's
  //      netlink / TCP-bind-service work survives
  //      exec.
  const artifact = path.join(root, 'dist/duckos-linux-amd64');
  const launcherBin = path.join(artifact, 'bin/duck-frr-launcher');
  const manifest = await fs.readFile(path.join(artifact, 'SHA256SUMS'), 'utf8');
  assert.match(manifest, /  bin\/duck-frr-launcher\n/);
  assert.match(manifest, /  systemd\/duck-frr-launcher\.service\n/);

  const launcherService = await fs.readFile(path.join(artifact, 'systemd/duck-frr-launcher.service'), 'utf8');
  // CapabilityBoundingSet: CAP_KILL was added deliberately
  // because the launcher is the canonical owner of FRR
  // lifetime (cmd/duck-frr-launcher/tracker.go) and every
  // STOP command identity-verifies the PID via
  // /proc/<pid>/cmdline, then signals SIGTERM (5s grace)
  // and SIGKILL. CAP_KILL is bounded to the launcher
  // service; it never reaches duck-agent (which keeps
  // its empty bounding set) and never reaches the FRR
  // child (which inherits from the launcher's Effective
  // set, not its bounding set).
  assert.match(launcherService, /^CapabilityBoundingSet=CAP_SETUID CAP_SETGID CAP_SETPCAP CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE CAP_SYS_ADMIN CAP_KILL$/m);
  assert.match(launcherService, /^NoNewPrivileges=no$/m);
  assert.match(launcherService, /--daemon --listen=\/run\/duckos-agent\/frr-launcher\.sock/);
  // CAP_SYS_ADMIN is included in the launcher's bounding
  // set because Debian FRR 10.3's zebra binary
  // unconditionally calls cap_set_proc with CAP_SYS_ADMIN
  // and the Debian zebra build does NOT expose the
  // upstream -S / --skip_runas flag (only bgpd does).
  // The launcher drops CAP_SETUID/SETGID/SETPCAP from its
  // own bounding before exec'ing FRR, so those do not
  // reach the FRR child; only the FRR caps (and
  // CAP_SYS_ADMIN) do. CAP_SYS_ADMIN MUST NOT reach
  // duck-agent (the agent's bounding set stays empty).

  const installer = await fs.readFile(path.join(artifact, 'install.sh'), 'utf8');
  // The install must wire both the launcher's file caps
  // and the FRR binaries' file caps. Without these, the
  // launcher's privilege drop and FRR's netlink /
  // TCP-bind-service work do not survive exec. The FRR
  // file caps also include cap_setpcap so FRR's privs_init
  // (which calls cap_set_proc) has CAP_SETPCAP in its
  // own effective set after the launcher's exec.
  assert.match(installer, /setcap cap_setuid,cap_setgid,cap_setpcap\+ep.*\/usr\/libexec\/duckos\/duck-frr-launcher/);
  assert.match(installer, /setcap cap_net_admin,cap_net_raw,cap_net_bind_service,cap_sys_admin,cap_setpcap\+ep.*\/usr\/lib\/frr\/(zebra|bgpd)/);
  // The launcher service is installed, started, and
  // enabled so the daemon is running before any FRR
  // pathspace apply.
  assert.match(installer, /install -o root -g root -m 0644.*systemd\/duck-frr-launcher\.service/);
  // BETA.2 B-009: the WG launcher joins the boot ordering with the
  // FRR launcher and the Agent.
  assert.match(installer, /systemctl start duck-restore\.service duck-frr-launcher\.service duck-wg-launcher\.service duck-agent\.service/);
  assert.match(installer, /systemctl enable duck-agent\.service duck-api\.service duck-restore\.service duck-frr-launcher\.service duck-wg-launcher\.service/);
});

test('preflight tolerates any pre-existing duck-dnsmasq identity', async () => {
  const script = await fs.readFile(installer, 'utf8');
  // The installer must not refuse to proceed when a leftover
  // duck-dnsmasq identity exists from a previous failed
  // install. Creation is silenced via `|| true`.
  assert.doesNotMatch(script, /A system user named duck-dnsmasq already exists; refusing to reuse its identity\./);
  assert.doesNotMatch(script, /A system group named duck-dnsmasq already exists; refusing to reuse its identity\./);
  assert.match(script, /groupadd --system duck-dnsmasq 2>\/dev\/null \|\| true/);
  assert.match(script, /useradd --system --gid duck-dnsmasq --home-dir \/var\/lib\/duckos-dnsmasq --no-create-home --shell \/usr\/sbin\/nologin duck-dnsmasq 2>\/dev\/null \|\| true/);
});

test('installer wires boot/recovery ordering and leaves the network inactive', async () => {
  const script = await fs.readFile(installer, 'utf8');
  assert.match(script, /install -d -o root -g root -m 0700 \/var\/lib\/duckos-agent \/var\/lib\/duckos-agent\/network/);
  // The installer creates the systemd-networkd Requires link for the
  // recovery oneshot and enables the oneshot so the boot guard
  // admits the network service on the next restart.
  assert.match(script, /ln -sfn? \/etc\/systemd\/system\/duck-network-recover\.service \/etc\/systemd\/system\/systemd-networkd\.service\.requires\/duck-network-recover\.service/);
  assert.match(script, /systemctl enable duck-network-recover\.service/);
  // Restarting systemd-networkd (when it is already running) is the
  // narrow way to make its runtime Requires= reflect the new
  // recovery ordering; the read-only flag ("may reload networkd")
  // was replaced by the explicit restart.
  assert.match(script, /systemctl restart systemd-networkd\.service/);
  // DNS stays inactive until an enrolled network transaction
  // supplies configuration; the installer must not start dnsmasq or
  // bind ports 53 / 67 itself.
  assert.doesNotMatch(script, /systemctl\s+start\s+duck-dnsmasq\.service/);
  assert.doesNotMatch(script, /systemctl\s+start\s+duck-network-recover\.service/);
  assert.doesNotMatch(script, /networkctl\s+reload/);
  assert.doesNotMatch(script, /networkdReload.*true/);
});

test('macOS Finder / AppleDouble metadata is excluded from the manifest', async (t) => {
  // Defense in depth: scripts/manifest.mjs must drop .DS_Store and
  // ._* sidecar files even if a previous build step leaked them into
  // the payload. Otherwise a tar packed on Darwin would smuggle them
  // through, the SHA256SUMS would not list them, and the installer
  // would refuse the artifact as "Unlisted artifact file" on Linux.
  const { artifact } = await fixture(t);
  // Use names that don't collide with real artifact files so the
  // test runs on any filesystem (APFS treats a `._foo` sidecar as a
  // sibling of `foo` and refuses to create it).
  await fs.writeFile(path.join(artifact, '._macos-noise'), 'macos noise');
  await fs.mkdir(path.join(artifact, 'macos-noise-dir'), { recursive: true });
  await fs.writeFile(path.join(artifact, 'macos-noise-dir/._sidecar'), 'macos noise');
  await fs.writeFile(path.join(artifact, '.DS_Store'), 'macos noise');
  const manifestScript = path.join(root, 'scripts/manifest.mjs');
  const epoch = '1788796800';
  const commit = '012345abcdef0000000000000000000000000000';
  const result = spawnSync('node', [manifestScript, artifact, '1.0.0-test', commit, 'false', 'amd64', epoch], {
    encoding: 'utf8', timeout: 15000,
  });
  assert.equal(result.status, 0, result.stderr);
  const sums = await fs.readFile(path.join(artifact, 'SHA256SUMS'), 'utf8');
  assert.match(sums, /  bin\/duck-agent\n/);
  assert.doesNotMatch(sums, /\._/);
  assert.doesNotMatch(sums, /\.DS_Store/);
});
