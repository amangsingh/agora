import React, { useEffect } from 'react';
import GraphCanvas from './features/graph/GraphCanvas';
import BlueprintEditor from './features/editor/BlueprintEditor';
import ExecutionPanel from './features/controls/ExecutionPanel';
import { useGraphStore } from './stores/graphStore';

function App() {
  const { addNode } = useGraphStore();

  // Initialize with some nodes for demo if empty?
  useEffect(() => {
    // Optional: Add default node if needed
  }, []);

  return (
    <div className="flex h-screen w-screen bg-slate-950 text-slate-200 overflow-hidden font-sans">

      {/* Left Sidebar: Code Editor (30%) */}
      <div className="w-[30%] border-r border-slate-700 flex flex-col">
        {/* Tools Palette / Header */}
        <div className="h-12 bg-slate-900 border-b border-slate-700 flex items-center px-4 justify-between">
          <span className="font-bold text-lg tracking-tight text-blue-400">
            AGORA <span className="text-slate-500 text-sm font-normal">v4.0</span>
          </span>
          <div className="flex gap-2">
            <button
              onClick={() => addNode('agent')}
              className="bg-slate-800 hover:bg-slate-700 text-xs px-2 py-1 rounded border border-slate-600 transition-colors"
            >
              + AGENT
            </button>
            <button
              onClick={() => addNode('tool')}
              className="bg-slate-800 hover:bg-slate-700 text-xs px-2 py-1 rounded border border-slate-600 transition-colors"
            >
              + TOOL
            </button>
          </div>
        </div>

        <div className="flex-1 overflow-hidden">
          <BlueprintEditor />
        </div>
      </div>

      {/* Right Area: Canvas + Execution (70%) */}
      <div className="flex-1 flex flex-col">

        {/* Top: Graph (70%) */}
        <div className="h-[70%] relative">
          <GraphCanvas />
          <div className="absolute top-4 left-4 pointer-events-none opacity-50">
            <h1 className="text-2xl font-bold bg-slate-950/50 p-2 rounded backdrop-blur-sm">
              The Foundry
            </h1>
          </div>
        </div>

        {/* Bottom: Execution Panel (30%) */}
        <div className="flex-1 border-t border-slate-700 overflow-hidden">
          <ExecutionPanel />
        </div>
      </div>

    </div>
  );
}

export default App;
