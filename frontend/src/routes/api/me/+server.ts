import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { GO_BACKEND_URL } from '$env/static/private';

export const GET: RequestHandler = async ({ cookies }) => {
  const jwt = cookies.get('jwt');
  
  if (!jwt) {
    return json({ error: 'Not authenticated' }, { status: 401 });
  }
  
  try {
    const res = await fetch(`${GO_BACKEND_URL}/api/me`, {
      headers: {
        'Authorization': `Bearer ${jwt}`
      }
    });
    
    if (!res.ok) {
      const error = await res.text();
      return json({ error }, { status: res.status });
    }
    
    const userData = await res.json();
    return json(userData);
  } catch (error) {
    console.error('Error fetching user data:', error);
    return json({ error: 'Failed to fetch user data' }, { status: 500 });
  }
}; 