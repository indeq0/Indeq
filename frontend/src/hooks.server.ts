// hooks server: handles session and authentication cookies
import type { Handle } from '@sveltejs/kit';
import { redirect } from '@sveltejs/kit';
import { verifyToken } from '$lib/server/auth';
import { APP_ENV } from '$env/static/private';

export const handle: Handle = async ({ event, resolve }) => {
  const jwt = event.cookies.get('jwt');

  const publicRoutes = [
    '/',
    '/login',
    '/register',
    '/enter-code',
    '/forgot-password',
    '/reset-password',
    '/terms',
    '/privacy',
    '/api/waitlist',
    '/sitemap.xml'
  ];
  const productionRoutes = ['/', '/terms', '/privacy', '/api/waitlist', '/sitemap.xml'];

  if (APP_ENV === 'PRODUCTION' && !productionRoutes.includes(event.url.pathname)) {
    return redirect(302, '/');
  }

  if (!publicRoutes.includes(event.url.pathname)) {
    const isValid = jwt && (await verifyToken(jwt));

    if (!isValid) {
      return redirect(302, '/login');
    }
  }

  return resolve(event);
};
