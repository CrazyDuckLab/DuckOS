import { createHash } from 'node:crypto';
import { readdir, readFile, writeFile, lstat } from 'node:fs/promises';
import { join, relative } from 'node:path';
const [dir, version, commit, dirty, arch, epoch] = process.argv.slice(2);
if (!dir || !/^[a-f0-9]{40}$/.test(commit) || !['amd64','arm64','armv7'].includes(arch)) throw new Error('Invalid build metadata');
await writeFile(join(dir, 'BUILD-INFO.json'), JSON.stringify({ version, commit, dirty: dirty === 'true', target: `linux/${arch}`, goVersion: 'go1.27.1', nodeVersion: process.version, sourceDateEpoch: Number(epoch), kind: 'internal-development' }, null, 2) + '\n');
async function inventory(path) {
  const result = [];
  for (const name of (await readdir(path)).sort()) {
    // Defense in depth: never include macOS Finder / AppleDouble metadata
    // in the artifact manifest, even if a previous build step leaked them.
    if (name === '.DS_Store' || name.startsWith('._')) continue;
    const full = join(path, name);
    const stat = await lstat(full);
    if (stat.isSymbolicLink()) throw new Error('Build payload must not contain symlinks');
    if (stat.isDirectory()) result.push(...await inventory(full));
    else if (stat.isFile()) result.push(full);
    else throw new Error('Build payload contains a special file');
  }
  return result;
}
const sums = [];
for (const file of await inventory(dir)) sums.push(`${createHash('sha256').update(await readFile(file)).digest('hex')}  ${relative(dir, file)}`);
await writeFile(join(dir, 'SHA256SUMS'), sums.join('\n') + '\n');
