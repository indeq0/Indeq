
export interface ConversationHeader {
  conversation_id: string;
  title: string;
  is_loading: boolean;
}

export interface ConversationSources {
  type: string;
  excerpt_number: number;
  title: string;
  file_path: string;
  file_url: string;
  extension: string;
}

export interface ConversationMessage {
  sender: 'user' | 'model';
  text: string;
  sources?: ConversationSources[];
  reasoning?: string[];
  model?: string;
}

export interface Conversation {
  conversation_id: string;
  title: string;
  full_messages: ConversationMessage[];
} 