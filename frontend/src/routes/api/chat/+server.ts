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

/**
 * DELETE /api/delete_conversation - Delete a conversation by ID
 */
export const DELETE: RequestHandler = async ({ request, cookies }) => {
  const { conversation_id } = await request.json();
  const session = cookies.get('jwt');
  if (!session) {
    return new Response(JSON.stringify({ error: 'Unauthorized' }), {
      status: 401,
      headers: { 'Content-Type': 'application/json' }
    });
  }

  try {
    const response = await fetch(`${GO_BACKEND_URL}/api/delete_conversation`, {
      method: 'POST',
      body: JSON.stringify({ conversation_id }),
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${session}`
      }
    });

    if (!response.ok) {
      const errorText = await response.text();
      return new Response(JSON.stringify({ error: errorText || 'Failed to delete conversation' }), {
        status: response.status,
        headers: { 'Content-Type': 'application/json' }
      });
    }

    return json({ success: true });
  } catch (err) {
    console.error('Error deleting conversation:', err);
    return new Response(JSON.stringify({ error: 'Failed to delete conversation' }), {
      status: 500,
      headers: { 'Content-Type': 'application/json' }
    });
  }
};