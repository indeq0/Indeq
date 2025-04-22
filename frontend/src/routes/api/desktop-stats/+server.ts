import { json } from '@sveltejs/kit';
import { GO_BACKEND_URL } from '$env/static/private';

export async function GET({ request, cookies }) {
  const session = cookies.get('jwt');
  if (!session) {
    return new Response(JSON.stringify({ error: 'Unauthorized' }), {
      status: 401,
      headers: { 'Content-Type': 'application/json' }
    });
  }
  
  try {
    const response = await fetch(`${GO_BACKEND_URL}/api/desktop_stats`, {
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${session}`
      }
    });
    
    const data = await response.json();
    return json(data);
  } catch (err) {
    return new Response(JSON.stringify({ error: 'Failed to fetch desktop stats' }), {
      status: 500,
      headers: { 'Content-Type': 'application/json' }
    });
  }
}