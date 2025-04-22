import { GO_BACKEND_URL } from '$env/static/private';
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { parseConversation } from '$lib/utils/chat';
import type { ChatMessage } from '$lib/types/chat';

export const load: PageServerLoad = async ({ params, cookies, fetch }) => {
    const id = params.id;
    const session = cookies.get('jwt');
    if (!session) {
      // No user, redirect to login
      throw redirect(302, '/login');
    }
    
    try {
      const conversation = await fetch(`/api/chat/${id}`, {
          method: 'GET',
          headers: {
              'Content-Type': 'application/json',
          },
      });

      // If the chat ID is invalid or not found, redirect to the main chat page
      if (!conversation.ok) {
        throw redirect(302, '/chat');
      }

      const integrations = await global.fetch(`${GO_BACKEND_URL}/api/integrations`, {
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${session}`
        }
      });
      const conversationData = await conversation.json();
      
      // Verify the conversation data has the expected structure
      if (!conversationData?.conversation?.title) {
        throw redirect(302, '/chat');
      }
      
      const integrationsData = await integrations.json();
      const providers = integrationsData.providers ?? [];

      const title = conversationData.conversation.title;
      let parsedConversation: ChatMessage[] = [];
      
      // Check if this navigation came from handleQuery
      const chatSource = cookies.get('chatSource');
      const newConversation = chatSource === 'query';
      parsedConversation = parseConversation(conversationData.conversation);
      
      // Clear the cookie after reading it
      if (newConversation) {
        cookies.delete('chatSource', { path: '/' });
      }
      
      return {
        id,
        title,
        conversation: parsedConversation,
        integrations: providers,
        newConversation
      };
    } catch (error) {
      // If any errors occur during API calls or data processing, redirect to the chat page
      console.error('Error loading chat:', error);
      throw redirect(302, '/chat');
    }
}