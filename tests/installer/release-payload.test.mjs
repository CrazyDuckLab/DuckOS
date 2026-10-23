import test from 'node:test';
import assert from 'node:assert/strict';
import { promises as fs } from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const required = process.env.DUCKOS_REQUIRE_RELEASE_PAYLOADS === '1';
const programs = ['duck-api', 'duck-agent', 'duck-updater', 'duck-restore', 'duck-frr-launcher', 'duck-wg-launcher', 'duck-nft-launcher'];
const targets = { amd64: [2, 0x3e], arm64: [2, 0xb7], armv7: [1, 0x28] };

for (const [arch, [elfClass, machine]] of Object.entries(targets)) {
  test(`genuine ${arch} release payload and artifact identity contract`, { skip: !required && 'Set DUCKOS_REQUIRE_RELEASE_PAYLOADS=1 after build-all; no emulated execution.' }, async (t) => {
    const dir = path.join(root, 'dist', `duckos-linux-${arch}`);
    const meta = JSON.parse(await fs.readFile(path.join(dir, 'BUILD-INFO.json'), 'utf8'));
    assert.equal(meta.version, 'v0.1.0-beta.3');
    assert.equal(meta.dirty, false);
    assert.equal(meta.target, `linux/${arch}`);
    assert.equal(meta.kind, 'prerelease');
    const head = spawnSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8' });
    assert.equal(head.status, 0); assert.equal(meta.commit, head.stdout.trim());
    assert.equal((await fs.readFile(path.join(dir, 'VERSION'), 'utf8')).trim(), meta.version);
    assert.deepEqual(await fs.readFile(path.join(dir, 'install.sh')), await fs.readFile(path.join(root, 'scripts/install.sh')));
    const sums = new Map();
    for (const row of (await fs.readFile(path.join(dir, 'SHA256SUMS'), 'utf8')).trim().split('\n')) {
      assert.match(row, /^[0-9a-f]{64}  [^\r\n]+$/);
      const name = row.slice(66); assert.equal(sums.has(name), false); sums.set(name, row.slice(0, 64));
      assert.equal(createHash('sha256').update(await fs.readFile(path.join(dir, name))).digest('hex'), row.slice(0, 64));
    }
    for (const program of programs) {
      const name = `bin/${program}`; const bytes = await fs.readFile(path.join(dir, name));
      assert.ok(sums.has(name), `${program} must be bound to the artifact manifest`);
      assert.equal(bytes[4], elfClass); assert.equal(bytes.readUInt16LE(18), machine);
      // Only the API/Agent expose buildinfo. Helper identity is intentionally
      // the artifact's version + full source SHA + per-file digest, not a new
      // executable version interface invented for this release.
      if (['duck-api', 'duck-agent'].includes(program)) {
        assert.ok(bytes.includes(Buffer.from(meta.version)), program);
        assert.ok(bytes.includes(Buffer.from(meta.commit)), program);
      }
    }
    const result = spawnSync('/bin/bash', [path.join(dir, 'install.sh'), '--artifact', dir, '--verify-artifact'], { encoding: 'utf8', timeout: 30000 });
    assert.equal(result.error, undefined); assert.equal(result.status, 0, result.stdout + result.stderr);
    const scratch = await fs.mkdtemp(path.join(os.tmpdir(), 'duckos-real-elf-'));
    t.after(() => fs.rm(scratch, { recursive: true, force: true }));
    const copy = path.join(scratch, 'artifact'); await fs.cp(dir, copy, { recursive: true });
    const binaryPath = path.join(copy, 'bin/duck-api'); const original = await fs.readFile(binaryPath);
    for (const mutation of ['truncate-file', 'maximum-section-offset']) {
      const bytes = mutation === 'truncate-file' ? original.subarray(0, Math.floor(original.length / 2)) : Buffer.from(original);
      if (mutation === 'maximum-section-offset') {
        if (elfClass === 1) bytes.writeUInt32LE(0xffffffff, 32);
        else bytes.writeBigUInt64LE(0xffffffffffffffffn, 40);
      }
      await fs.writeFile(binaryPath, bytes);
      const originalManifest = await fs.readFile(path.join(dir, 'SHA256SUMS'), 'utf8');
      const digest = createHash('sha256').update(bytes).digest('hex');
      await fs.writeFile(path.join(copy, 'SHA256SUMS'), originalManifest.replace(/^[0-9a-f]{64}  bin\/duck-api$/m, `${digest}  bin/duck-api`));
      const denied = spawnSync('/bin/bash', [path.join(copy, 'install.sh'), '--artifact', copy, '--verify-artifact'], { encoding: 'utf8', timeout: 30000 });
      assert.equal(denied.error, undefined); assert.notEqual(denied.status, 0, `${arch} ${mutation}`);
      assert.match(denied.stderr, /ELF .*bounds.*truncated binary/, `${arch} ${mutation}: ${denied.stderr}`);
    }
  });
}
