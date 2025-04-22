// hooks server: handles session and authentication cookies
import type { Handle } from '@sveltejs/kit';
import { redirect } from '@sveltejs/kit';
import { verifyToken } from '$lib/server/auth';
import { APP_ENV } from '$env/static/private';

export const handle: Handle = async ({ event, resolve }) => {
  const jwt = event.cookies.get('jwt');
  const betaPassed = event.cookies.get('betaPassed') === 'true';
  const isAuthenticated = jwt && (await verifyToken(jwt));

  if (APP_ENV === 'PRODUCTION') {
    const alwaysAllowed = ['/', '/login', '/beta-code'];
    if (!alwaysAllowed.includes(event.url.pathname)) {
      if (!(event.url.pathname === '/register' && betaPassed)) {
        return redirect(302, '/');
      }
    }
  }

  if (isAuthenticated && ['/login', '/register'].includes(event.url.pathname) && event.request.method === 'GET') {
    return redirect(302, '/chat');
  }

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
    '/sitemap.xml',
    '/sso/GOOGLE',
    '/sso/GOOGLE/callback'
  ];

  if (!publicRoutes.includes(event.url.pathname)) {
    if (!isAuthenticated) {
      return redirect(302, '/login');
    }
  }

  return resolve(event);
};

