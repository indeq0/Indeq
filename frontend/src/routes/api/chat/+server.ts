import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { GO_BACKEND_URL } from '$env/static/private';

/**
 * GET /api/chat - Get all conversation history for the current user
 */
export const GET: RequestHandler = async ({ cookies }) => {
  const session = cookies.get('jwt');
  if (!session) {
    return new Response(JSON.stringify({ error: 'Unauthorized' }), {
      status: 401,
      headers: { 'Content-Type': 'application/json' }
    });
  }
  
  try {
    const response = await fetch(`${GO_BACKEND_URL}/api/get_all_conversations`, {
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${session}`
      }
    });
    
    if (!response.ok) {
      const errorText = await response.text();
      return new Response(JSON.stringify({ error: errorText || 'Failed to fetch conversations' }), {
        status: response.status,
        headers: { 'Content-Type': 'application/json' }
      });
    }
    
    const data = await response.json();
    return json(data);
  } catch (err) {
    console.error('Error fetching conversations:', err);
    return new Response(JSON.stringify({ error: 'Failed to fetch conversations' }), {
      status: 500,
      headers: { 'Content-Type': 'application/json' }
    });
  }
}; 