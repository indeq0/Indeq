import type { ChatMessage, ChatState, ChatSource } from "$lib/types/chat";
import type { Conversation } from "$lib/types/conversation";

export function processSource(payload: any, botMessage: ChatMessage) {
    const sourceId = payload.excerpt_number;
    const sourceExtension = payload.extension;
    const sourceFilePath = payload.file_path;
    const sourceTitle = payload.title;

    const source : ChatSource = {
        id: sourceId,
        extension: sourceExtension.toLowerCase(),
        filePath: sourceFilePath,
        title: sourceTitle,
        showTooltip: true,
    }

    botMessage.sources.push(source);
}

// Function to process reasoning messages and update message state
export function processReasoningMessage(data: string, botMessage: ChatMessage, state: ChatState) {
  // Handle reasoning paragraph break
  if (/\n\n/.test(data) && botMessage.reasoning.length > 0) {
    botMessage.reasoning[botMessage.reasoning.length - 1].collapsed = true;
    botMessage.reasoning.push({ text: '', collapsed: false });
    return;
  }

  // Skip <think> tag or reasoning paragraph break
  if (/\u003cthink\u003e/.test(data) || /\n\n/.test(data)) {
    return;
  }

  // Handle </think> tag
  if (/\u003c\/think\u003e/.test(data)) {
    state.isReasoning = false;

    // Auto-collapse reasoning section when reasoning is complete
    botMessage.reasoningSectionCollapsed = true;

    if (botMessage.reasoning.length > 0) {
      botMessage.reasoning[botMessage.reasoning.length - 1].collapsed = true;
    }
    state.messages = [...state.messages.slice(0, -1), botMessage];

    return;
  }

  // Add or update reasoning text
  if (botMessage.reasoning.length > 0) {
    botMessage.reasoning[botMessage.reasoning.length - 1].text += data;
  } else {
    botMessage.reasoning.push({ text: data, collapsed: false });
  }

  preserveReasoningSectionState(botMessage, state);

  // Update messages array
  if (state.messages[state.messages.length - 1].sender === 'bot') {
    state.messages[state.messages.length - 1].reasoning = botMessage.reasoning;
    state.messages[state.messages.length - 1].reasoningSectionCollapsed =
      botMessage.reasoningSectionCollapsed;
  } else {
    state.messages = [...state.messages, botMessage];
  }
}

// Function to process output message and update message state
export function processOutputMessage(data: string, botMessage: ChatMessage, state: ChatState) {
  botMessage.text += data;
  preserveReasoningSectionState(botMessage, state);
  state.messages = [...state.messages.slice(0, -1), botMessage];
}

// Function to preserve reasoningSectionCollapsed property
function preserveReasoningSectionState(botMessage: ChatMessage, state: ChatState): void {
  if (state.messages.length > 0 && state.messages[state.messages.length - 1].sender === 'bot') {
    const currentBotMessage = state.messages[state.messages.length - 1];
    botMessage.reasoningSectionCollapsed = currentBotMessage.reasoningSectionCollapsed;
  }
}

// Function to toggle reasoning visibility
export function toggleReasoning(messageIndex: number, reasoningIndex: number, state: ChatState) {
  const lastMessage = state.messages[messageIndex];
  
  lastMessage.reasoning[reasoningIndex].collapsed =
    !lastMessage.reasoning[reasoningIndex].collapsed;
  state.messages = [...state.messages]; // Trigger reactivity
}

// Function to parse conversation payload into ChatMessage[]
export function parseConversation(conversation: Conversation): ChatMessage[] {
  const messages: ChatMessage[] = [];
  if(!conversation.full_messages) {
    return messages;
  }

  for (const message of conversation.full_messages) {
    if (message.sender === "user") {
      messages.push({
        text: message.text,
        sender: message.sender,
        reasoning: [],
        reasoningSectionCollapsed: false,
        sources: []
      })
    } else {
      const reasoning = [];

      if (message.reasoning) {
        for (const thought of message.reasoning) 
          if (thought.length > 0 && thought !== "<think>" && thought !== "</think>") {
            reasoning.push({
              text: thought,
              collapsed: true,
            })
          }
      }

      // Ensure text is properly processed when loading conversations
      const text = message.text.toString();
      messages.push({
        text: text,
        sender: message.sender,
        reasoning: reasoning,
        reasoningSectionCollapsed: true,
        sources: message.sources ? message.sources.map((source) => ({
          id: source.excerpt_number,
          extension: source.extension,
          filePath: source.file_path,
          title: source.title,
          showTooltip: true
        })) : []
      })
    }
  }

  return messages;
}