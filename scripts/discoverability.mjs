// Pure metadata preparation. No network requests, publication or filesystem writes.
export function privateMetadata(project) {
  if (project.schemaVersion !== 1 || project.publicLaunchApproved !== false ||
      project.indexingEnabled !== false || project.releaseVersion !== null ||
      project.canonicalSiteURL !== null || project.license !== null) {
    throw new Error('Public launch metadata requires a separate reviewed release change');
  }
  if (project.topics.length > 20 || new Set(project.topics).size !== project.topics.length ||
      project.topics.some(topic => !/^[a-z0-9-]{1,50}$/.test(topic))) {
    throw new Error('Invalid repository topics');
  }
  for (const [route, source] of Object.entries(project.routes)) {
    if (!/^\/(?:[a-z0-9-]+)?$/.test(route) ||
        !/^[a-zA-Z0-9_./-]+\.md$/.test(source) ||
        source.startsWith('/') || source.split('/').some(p => p === '..' || p === '.')) {
      throw new Error('Invalid documentation route');
    }
  }
  return {
    headers: { 'X-Robots-Tag': 'noindex, nofollow, noarchive' },
    robots: 'User-agent: *\nDisallow: /\n',
    openGraph: { 'og:title': project.name, 'og:description': project.description, 'og:type': 'website' },
    structuredData: {
      '@context': 'https://schema.org',
      '@type': 'SoftwareSourceCode',
      name: project.name,
      description: project.description,
      codeRepository: project.repository,
      programmingLanguage: ['Go', 'TypeScript'],
      creativeWorkStatus: 'In development',
      targetProduct: {
        '@type': 'SoftwareApplication', name: project.name,
        applicationCategory: 'Networking software', operatingSystem: 'Linux (supported Debian)',
      },
    },
    sitemap: null,
    releaseFeed: null,
  };
}

// Preview URLs are deployment inputs, never inferred to be the project's public
// canonical identity. Artifacts retain noindex and contain no release entries.
export function previewArtifacts(project, previewOrigin, updated) {
  const metadata = privateMetadata(project);
  const origin = new URL(previewOrigin);
  if (origin.protocol !== 'https:' || origin.username || origin.password ||
      origin.pathname !== '/' || origin.search || origin.hash ||
      !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/.test(updated) ||
      !Number.isFinite(Date.parse(updated))) {
    throw new Error('Invalid preview origin or metadata timestamp');
  }
  const escape = value => value.replaceAll('&', '&amp;').replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;').replaceAll('"', '&quot;').replaceAll("'", '&apos;');
  const absolute = path => escape(new URL(path, origin).href);
  return {
    ...metadata,
    sitemap: '<?xml version="1.0" encoding="UTF-8"?>\n' +
      '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n' +
      Object.keys(project.routes).sort().map(route => `  <url><loc>${absolute(route)}</loc></url>`).join('\n') +
      '\n</urlset>\n',
    releaseFeed: '<?xml version="1.0" encoding="UTF-8"?>\n' +
      '<feed xmlns="http://www.w3.org/2005/Atom">\n' +
      `<id>${absolute('/feed.atom')}</id><title>DuckOS — no public releases</title>\n` +
      `<updated>${updated}</updated><author><name>${escape(project.organization)}</name></author>\n` +
      `<link rel="self" href="${absolute('/feed.atom')}"/>\n</feed>\n`,
  };
}
