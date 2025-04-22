import type { RequestHandler } from '@sveltejs/kit';

interface SitemapEntry {
  path: string;
  lastmod: string;
  changefreq: 'always' | 'hourly' | 'daily' | 'weekly' | 'monthly' | 'yearly' | 'never';
  priority: string;
}

export const GET: RequestHandler = async () => {
  const baseUrl = 'https://indeq.app';
  const pages: SitemapEntry[] = [
    { path: '/', lastmod: '2025-03-06', changefreq: 'monthly', priority: '1.0' },
    { path: '/privacy', lastmod: '2025-03-06', changefreq: 'monthly', priority: '1.0' },
    { path: '/terms', lastmod: '2025-03-06', changefreq: 'monthly', priority: '1.0' }
    // TODO: Add these back in on launch
    // { path: '/chat', lastmod: '2025-03-06', changefreq: 'monthly', priority: '1.0' },
    // { path: '/login', lastmod: '2025-03-06', changefreq: 'monthly', priority: '1.0' },
    // { path: '/register', lastmod: '2025-03-06', changefreq: 'monthly', priority: '1.0' },
  ];

  const xml = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  ${pages
    .map(
      (page) => `
    <url>
      <loc>${baseUrl}${page.path}</loc>
      <lastmod>${page.lastmod}</lastmod>
      <changefreq>${page.changefreq}</changefreq>
      <priority>${page.priority}</priority>
    </url>`
    )
    .join('')}
</urlset>`;

  return new Response(xml, {
    headers: {
      'Content-Type': 'application/xml',
      'Cache-Control': 'public, max-age=86400' // Cache for 1 day
    }
  });
};
