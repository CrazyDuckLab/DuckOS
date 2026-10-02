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
  const files = new Map([
    ['VERSION', '1.0.0-dev.012345abcdef\n'],
    ['BUILD-INFO.json', JSON.stringify({ version: '1.0.0-dev.012345abcdef', commit: '012345abcdef0000000000000000000000000000', dirty: false, target: 'linux/amd64', goVersion: 'go1.27.1', nodeVersion: 'v24.20.0', sourceDateEpoch: 1788796800, kind: 'internal-development' }, null, 2) + '\n'],
    ['install.sh', await fs.readFile(installer)],
    ['bin/duck-agent', elf],
    ['bin/duck-api', elf],
    ['bin/duck-updater', elf],
    ['bin/duck-restore', elf],
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
  const result = spawnSync('/bin/bash', [installer, '--artifact', artifact, '--verify-artifact', ...args], {
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
  rejected(verify(f.artifact), /installer belonging to this exact artifact/);
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
  const result = spawnSync('/bin/bash', [installer, '--artifact', artifact, '--check'], { encoding: 'utf8', timeout: 15000 });
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
  // The Agent keeps a deliberately narrow capability set:
  // CAP_NET_ADMIN for routing / addressing, CAP_NET_RAW for FRR's
  // raw socket needs. CAP_SYS_ADMIN must NEVER appear here — that
  // is the privilege separation invariant guarded by the FRR fix
  // (see internal/frr/privilege_model_test.go for the Go side).
  assert.match(agent, /^CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_SETGID CAP_SETUID CAP_SYS_ADMIN CAP_NET_BIND_SERVICE$/m);
  assert.match(agent, /^AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW CAP_SETGID CAP_SETUID CAP_SYS_ADMIN CAP_NET_BIND_SERVICE$/m);
  // CAP_SYS_ADMIN is required by FRR 10.x's privs_init; the
  // Agent does not USE it (only FRR's startup must be admitted).
  // Documenting that the Agent must not GROW beyond this set is
  // the privilege-model invariant.
  // The agent owns the FRR pathspace (/var/lib/frr/<ps>,
  // /run/frr/<ps>). The Debian FRR package ships /var/lib/frr
  // owned frr:frr 0755 and systemd's CapabilityBoundingSet strips
  // CAP_DAC_OVERRIDE so User=root alone cannot write into
  // /var/lib/frr. Granting SupplementaryGroups is the narrow
  // standard way to give the agent the group membership it needs
  // without expanding its capability set.
  assert.match(agent, /^SupplementaryGroups=frr frrvty$/m);
  assert.match(agent, /--allowed-uid @DUCKOS_UID@ --environment test/);
  assert.match(agent, /^RuntimeDirectoryMode=0751$/m);
  assert.match(agent, /^PrivateNetwork=no$/m);
  for (const unit of [api, agent]) {
    assert.match(unit, /^NoNewPrivileges=yes$/m);
    assert.match(unit, /^ProtectSystem=strict$/m);
    assert.match(unit, /^MemoryMax=256M$/m);
    assert.match(unit, /^TasksMax=64$/m);
    assert.match(unit, /^LimitCORE=0$/m);
  }
});

test('preflight tolerates a regular human admin named duck-dnsmasq', async () => {
  const script = await fs.readFile(installer, 'utf8');
  // The installer must not blanket-refuse every existing duck-dnsmasq
  // identity — a leftover system user from a previous failed install
  // still needs to be cleaned up by the operator, but a regular human
  // account (uid/gid >= 1000) sharing the name must not block the
  // install. Mirror the duckos preflight behaviour exactly.
  assert.doesNotMatch(script, /! getent passwd duck-dnsmasq >\/dev\/null \|\| fail 'The duck-dnsmasq user already exists; refusing to reuse its identity\./);
  assert.doesNotMatch(script, /! getent group duck-dnsmasq >\/dev\/null \|\| fail 'The duck-dnsmasq group already exists; refusing to reuse its identity\./);
  assert.match(script, /if getent passwd duck-dnsmasq >\/dev\/null; then/);
  assert.match(script, /A system user named duck-dnsmasq already exists; refusing to reuse its identity\./);
  assert.match(script, /if getent group duck-dnsmasq >\/dev\/null; then/);
  assert.match(script, /A system group named duck-dnsmasq already exists; refusing to reuse its identity\./);
  // Creation must not hard-fail when a human duck-dnsmasq already
  // exists; the same tolerate-existing pattern that the duckos group
  // and useradd lines use applies here too.
  assert.match(script, /groupadd --system duck-dnsmasq 2>\/dev\/null \|\| \[\[ \$\(getent group duck-dnsmasq \| cut -d: -f3\) -ge 1000 \]\] \|\| fail 'Could not create duck-dnsmasq group;/);
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
