export interface RunRequest {
    graph: any; // We send the full Blueprint or Graph JSON
    input: string;
}

export interface RunResponse {
    history: ChatMessage[];
    final_state: any;
}

export interface ChatMessage {
    role: string;
    content: string;
    tool_calls?: ToolCall[];
}

export interface ToolCall {
    id: string;
    type: string;
    function: {
        name: string;
        arguments: any;
    };
}
