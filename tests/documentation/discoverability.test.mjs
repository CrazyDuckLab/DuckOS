import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { privateMetadata, previewArtifacts } from '../../scripts/discoverability.mjs';

const root = new URL('../../', import.meta.url);
const project = () => JSON.parse(readFileSync(new URL('docs/launch/project.json', root), 'utf8'));

test('launch metadata stays private without invented release or license', () => {
  const m = privateMetadata(project());
  assert.match(m.headers['X-Robots-Tag'], /noindex/);
  assert.equal(m.robots, 'User-agent: *\nDisallow: /\n');
  assert.equal(m.sitemap, null);
  assert.equal(m.releaseFeed, null);
  assert.equal(m.structuredData.softwareVersion, undefined);
  assert.equal(m.structuredData.license, undefined);
  assert.equal(m.openGraph['og:url'], undefined);
});

test('publication flags cannot accidentally enable indexing', () => {
  for (const [field, value] of Object.entries({ publicLaunchApproved: true,
    indexingEnabled: true, releaseVersion: '1.0.0', canonicalSiteURL: 'https://example.test', license: 'unreviewed' })) {
    const p = project(); p[field] = value;
    assert.throws(() => privateMetadata(p));
  }
});

test('all site routes resolve to repository documents', () => {
  const p = project(); privateMetadata(p);
  for (const source of Object.values(p.routes)) {
    assert.ok(existsSync(new URL(source, root)), source);
  }
  assert.ok(p.routes['/wireguard'] && p.routes['/subscriptions'] && p.routes['/edge-node']);
});

test('metadata rejects topic overflow and path escapes', () => {
  const p = project(); p.topics.push('overflow');
  assert.throws(() => privateMetadata(p));
  for (const source of ['../private.md', '/private.md', 'docs/../private.md']) {
    const q = project(); q.routes['/test'] = source;
    assert.throws(() => privateMetadata(q));
  }
});

test('discovery map uses real local documentation links', () => {
  const content = readFileSync(new URL('llms.txt', root), 'utf8');
  for (const match of content.matchAll(/\]\(([^)]+)\)/g)) {
    assert.ok(existsSync(fileURLToPath(new URL(match[1], root))), match[1]);
  }
});

test('preview sitemap and empty Atom feed retain private indexing policy', () => {
  const m = previewArtifacts(project(), 'https://docs.example.invalid', '2026-09-09T00:00:00Z');
  assert.match(m.sitemap, /https:\/\/docs.example.invalid\/wireguard/);
  assert.equal((m.sitemap.match(/<url>/g) || []).length, Object.keys(project().routes).length);
  assert.match(m.releaseFeed, /no public releases/);
  assert.doesNotMatch(m.releaseFeed, /<entry>/);
  assert.match(m.headers['X-Robots-Tag'], /noindex/);
  assert.match(m.robots, /Disallow: \//);
  assert.equal(m.openGraph['og:url'], undefined);
});

test('preview origins cannot carry credentials or query secrets', () => {
  for (const origin of ['http://example.invalid', 'https://user:secret@example.invalid',
    'https://example.invalid/?token=secret', 'https://example.invalid/subpath']) {
    assert.throws(() => previewArtifacts(project(), origin, '2026-09-09T00:00:00Z'));
  }
});
