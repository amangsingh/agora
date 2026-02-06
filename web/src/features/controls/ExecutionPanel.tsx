import React, { useState } from 'react';
import { Play, Square, Terminal } from 'lucide-react';
import { useGraphStore } from '../../stores/graphStore';
import { client } from '../../api/client';
import { ChatMessage } from '../../types/api';

const ExecutionPanel = () => {
    const { syncToBlueprint } = useGraphStore();
    const [logs, setLogs] = useState<ChatMessage[]>([]);
    const [isRunning, setIsRunning] = useState(false);
    const [input, setInput] = useState('Hello Agent');

    const handleRun = async () => {
        setIsRunning(true);
        setLogs([]); // Clear logs

        try {
            const blueprint = syncToBlueprint();
            // In a real app we'd convert Blueprint to the specific Graph JSON format expected by API
            // For now, assuming API accepts Blueprint-like structure or we map it. 
            // The API expects `ModelRequest` logic in `agora-server`... 
            // WAIT: `POST /run` in `agora-server` expects... let's check handler.go logic.
            // handler.go decodes `RunRequest`. 
            // We defined `RunRequest` in `api.ts` as { graph: any, input: string }.

            const req = {
                graph: blueprint,
                input: input
            };

            const res = await client.runAgent(req);
            setLogs(res.history);
        } catch (error) {
            console.error(error);
            setLogs([{ role: 'system', content: `Error: ${error}` }]);
        } finally {
            setIsRunning(false);
        }
    };

    return (
        <div className="h-full flex flex-col bg-slate-900 border-t border-slate-700">
            {/* Toolbar */}
            <div className="flex items-center justify-between p-2 bg-slate-800 border-b border-slate-700">
                <div className="flex items-center gap-2">
                    <Terminal size={14} className="text-slate-400" />
                    <span className="text-xs font-mono text-slate-300">TERMINAL</span>
                </div>
                <div className="flex gap-2">
                    <input
                        type="text"
                        value={input}
                        onChange={(e) => setInput(e.target.value)}
                        className="bg-slate-900 border border-slate-600 rounded px-2 py-1 text-xs text-white w-64 focus:outline-none focus:border-blue-500"
                        placeholder="User Input..."
                    />
                    <button
                        onClick={handleRun}
                        disabled={isRunning}
                        className={`flex items-center gap-1 px-3 py-1 rounded text-xs font-bold transition-colors ${isRunning
                                ? 'bg-slate-700 text-slate-500 cursor-not-allowed'
                                : 'bg-green-600 hover:bg-green-500 text-white'
                            }`}
                    >
                        {isRunning ? <Square size={10} className="animate-pulse" /> : <Play size={10} />}
                        {isRunning ? 'RUNNING' : 'RUN'}
                    </button>
                </div>
            </div>

            {/* Logs Area */}
            <div className="flex-1 overflow-auto p-4 font-mono text-sm space-y-2">
                {logs.length === 0 && (
                    <div className="text-slate-600 italic">Ready for execution...</div>
                )}
                {logs.map((msg, i) => (
                    <div key={i} className="flex gap-2">
                        <span className={`uppercase text-xs font-bold w-16 shrink-0 ${msg.role === 'user' ? 'text-blue-400' :
                                msg.role === 'system' ? 'text-red-400' :
                                    'text-green-400'
                            }`}>
                            {msg.role}
                        </span>
                        <span className="text-slate-300 whitespace-pre-wrap">{msg.content}</span>
                    </div>
                ))}
            </div>
        </div>
    );
};

export default ExecutionPanel;
