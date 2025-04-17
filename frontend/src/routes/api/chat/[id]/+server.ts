import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { GO_BACKEND_URL } from '$env/static/private';

/**
 * GET /api/chat/[id] - Get a specific conversation by ID
 */
export const GET: RequestHandler = async ({ cookies, params }) => {
  const session = cookies.get('jwt');
  if (!session) {
    return new Response(JSON.stringify({ error: 'Unauthorized' }), {
      status: 401,
      headers: { 'Content-Type': 'application/json' }
    });
  }
  
  const conversationId = params.id;
  
  try {
    const response = await fetch(`${GO_BACKEND_URL}/api/get_conversation_history`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${session}`
      },
      body: JSON.stringify({
        conversation_id: conversationId
      })
    });
    
    if (!response.ok) {
      const errorText = await response.text();
      return new Response(JSON.stringify({ error: errorText || 'Failed to fetch conversation' }), {
        status: response.status,
        headers: { 'Content-Type': 'application/json' }
      });
    }
    
    const data = await response.json();
    return json(data);
  } catch (err) {
    console.error('Error fetching conversation:', err);
    return new Response(JSON.stringify({ error: 'Failed to fetch conversation' }), {
      status: 500,
      headers: { 'Content-Type': 'application/json' }
    });
  }
}; 