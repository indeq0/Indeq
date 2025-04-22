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
    fetchConversations: async (newConversation: boolean = false) => {
      update(state => ({ ...state, loading: true, error: null }));
      
      try {
        const response = await fetch('/api/chat');
        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || 'Failed to fetch conversations');
        }
        
        const data = await response.json();
        if(data.conversation_headers){
            update(state => {
              // Get the current headers to preserve loading states
              const currentHeaders = state.headers;
              
              // Process the new headers
              const newHeaders = data.conversation_headers.map((header: ConversationPayload, index: number) => {
                // Check if this conversation was previously in a loading state
                const existingHeader = currentHeaders.find(h => h.conversation_id === header.conversation_id);
                
                // Only apply loading state to new conversations if specifically requested
                const isLoading = newConversation && index === data.conversation_headers.length - 1 
                  ? true 
                  : existingHeader?.is_loading || false;
                
                return {
                  conversation_id: header.conversation_id,
                  title: header.title,
                  is_loading: isLoading
                };
              }).reverse();
              
              return { 
                ...state, 
                headers: newHeaders,
                loading: false 
              };
            });
        }
        
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : 'Failed to fetch conversations';
        update(state => ({ ...state, loading: false, error: errorMessage }));
        console.error('Error fetching conversations:', err);
      }
    },
    // Fetch and update only a specific conversation's title
    updateConversationTitle: async (conversationId: string) => {
      try {
        const response = await fetch('/api/chat');
        if (!response.ok) {
          throw new Error('Failed to fetch conversations');
        }
        
        const data = await response.json();
        if (data.conversation_headers) {
          const targetConversation = data.conversation_headers.find(
            (header: ConversationPayload) => header.conversation_id === conversationId
          );
          
          if (targetConversation) {
            // First set loading to false, but keep the old title
            update(state => ({
              ...state,
              headers: state.headers.map(header => 
                header.conversation_id === conversationId
                  ? { ...header, is_loading: false }
                  : header
              )
            }));
            
            // Wait a short moment to ensure loading state change has been processed
            // Then update the title to trigger the animation
            setTimeout(() => {
              update(state => ({
                ...state,
                headers: state.headers.map(header => 
                  header.conversation_id === conversationId
                    ? { ...header, title: targetConversation.title }
                    : header
                )
              }));
            }, 150); // Shorter delay to avoid feeling sluggish but still allow state to update
          }
        }
      } catch (err) {
        console.error('Error updating conversation title:', err);
      }
    },
    // Update a specific conversation's status (used when streaming completes)
    updateConversationStatus: (conversationId: string, isLoading: boolean) => {
      update(state => ({
        ...state,
        headers: state.headers.map(header => 
          header.conversation_id === conversationId
            ? { ...header, is_loading: isLoading }
            : header
        )
      }));
    },
    deleteConversation: async (conversationId: string) => {
      try {
        const response = await fetch(`/api/chat`, {
          method: 'DELETE',
          body: JSON.stringify({ conversation_id: conversationId })
        });
        
        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || 'Failed to delete conversation');
        }
        
        // Update the local state by removing the deleted conversation
        update(state => ({
          ...state,
          headers: state.headers.filter(header => header.conversation_id !== conversationId)
        }));
        
      } catch (err) {
        console.error('Error deleting conversation:', err);
      }
    },
    clear: () => {
      set({ headers: [], loading: false, error: null });
    }
  };
};

export const conversationStore = createConversationStore(); 