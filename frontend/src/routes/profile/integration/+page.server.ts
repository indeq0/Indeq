import type { PageServerLoad } from './$types';
import { GO_BACKEND_URL } from '$env/static/private';
import { error, redirect } from '@sveltejs/kit';

export const load: PageServerLoad = async ({ cookies, fetch }) => {
  const token = cookies.get('jwt');
  if (!token) {
    throw redirect(302, '/login');
  }

  let connectedProviders: string[] = [];
  try {
    const response = await fetch(`${GO_BACKEND_URL}/api/integrations`, {
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`
      }
    });
    if (!response.ok) {
      throw error(500, 'Failed to fetch integrations');
    }
    const data = await response.json();
    connectedProviders = data.providers ?? [];
  } catch (err) {
    throw error(500, 'Failed to fetch integrations');
  }

  return {
    connectedProviders
  };
};
