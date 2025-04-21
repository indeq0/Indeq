import { writable } from 'svelte/store';
import type { ConversationHeader } from '$lib/types/conversation';

type ConversationHistoryState = {
  headers: ConversationHeader[];
  loading: boolean;
  error: string | null;
};

type ConversationPayload = {
  conversation_id: string;
  title: string;
}

const createConversationStore = () => {
  const { subscribe, set, update } = writable<ConversationHistoryState>({
    headers: [],
    loading: false,
    error: null
  });

  return {
    subscribe,
    fetchConversations: async () => {
      update(state => ({ ...state, loading: true, error: null }));
      
      try {
        const response = await fetch('/api/chat');
        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || 'Failed to fetch conversations');
        }
        
        const data = await response.json();
        if(data.conversation_headers){
            update(state => ({ 
                ...state, 
                headers: data.conversation_headers.map((header: ConversationPayload) => ({
                  conversation_id: header.conversation_id,
                  title: header.title
                })).reverse() || [],
                loading: false 
              }));
        }
        
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : 'Failed to fetch conversations';
        update(state => ({ ...state, loading: false, error: errorMessage }));
        console.error('Error fetching conversations:', err);
      }
    },
    clear: () => {
      set({ headers: [], loading: false, error: null });
    }
  };
};

export const conversationStore = createConversationStore(); 