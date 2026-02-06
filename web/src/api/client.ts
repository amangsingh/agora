import { RunRequest, RunResponse, ChatMessage } from '../types/api';

const BASE_URL = 'http://localhost:8080';

export class AgoraClient {
    private token: string;

    constructor(token: string) {
        this.token = token;
    }

    async runAgent(request: RunRequest): Promise<RunResponse> {
        const response = await fetch(`${BASE_URL}/run`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${this.token}`,
            },
            body: JSON.stringify(request),
        });

        if (!response.ok) {
            throw new Error(`Failed to run agent: ${response.statusText}`);
        }

        return response.json();
    }

    async getHistory(): Promise<ChatMessage[]> {
        const response = await fetch(`${BASE_URL}/history`, {
            headers: {
                'Authorization': `Bearer ${this.token}`,
            },
        });

        if (!response.ok) {
            throw new Error(`Failed to fetch history: ${response.statusText}`);
        }

        return response.json();
    }
}

// Singleton or hook usage
export const client = new AgoraClient('agora-dev-token'); // Hardcoded dev token for now
