export interface ChatSource {
    id: number;
    extension: string;
    filePath: string;
    title: string;
    showTooltip: boolean;
}

export interface ChatMessage {
    text: string;
    sender: string;
    reasoning: {text: string; collapsed: boolean}[];
    reasoningSectionCollapsed: boolean;
    sources: ChatSource[];
    sourcesScrollAtEnd?: boolean;
    isScrollable?: boolean;
}

export interface ChatState {
    messages: ChatMessage[];
    isReasoning: boolean;
}