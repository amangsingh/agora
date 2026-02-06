import React from 'react';
import { Handle, Position, NodeProps } from '@xyflow/react';
import { Bot, Wrench, ChevronRight } from 'lucide-react';
import { NodeData } from '../../stores/graphStore';

export const AgentNode = ({ data }: NodeProps<NodeData>) => {
    return (
        <div className="bg-slate-900 border-2 border-slate-700 rounded-xl shadow-xl min-w-[200px]">
            <div className="flex items-center gap-2 p-3 border-b border-slate-700 bg-slate-800 rounded-t-xl">
                <Bot size={18} className="text-blue-400" />
                <span className="font-bold text-slate-200">{data.label}</span>
            </div>
            <div className="p-3 text-sm text-slate-400">
                <div className="flex items-center gap-2 mb-2">
                    <span className="text-xs uppercase font-mono text-slate-500">Model</span>
                    <span className="text-slate-300">llama3</span>
                </div>
                <div className="text-xs text-slate-500 line-clamp-2">
                    System Prompt configured...
                </div>
            </div>

            {/* Handles */}
            <Handle type="target" position={Position.Top} className="!bg-blue-500 !w-3 !h-3" />
            <Handle type="source" position={Position.Bottom} className="!bg-blue-500 !w-3 !h-3" />
        </div>
    );
};

export const ToolNode = ({ data }: NodeProps<NodeData>) => {
    return (
        <div className="bg-slate-900 border-2 border-emerald-700/50 rounded-xl shadow-xl min-w-[180px]">
            <div className="flex items-center gap-2 p-3 border-b border-emerald-900/50 bg-emerald-900/20 rounded-t-xl">
                <Wrench size={18} className="text-emerald-400" />
                <span className="font-bold text-emerald-100">{data.label}</span>
            </div>
            <div className="p-3 text-sm text-slate-400">
                <div className="text-xs text-emerald-500/70">Tool Executor</div>
            </div>

            {/* Handles */}
            <Handle type="target" position={Position.Top} className="!bg-emerald-500 !w-3 !h-3" />
            <Handle type="source" position={Position.Bottom} className="!bg-emerald-500 !w-3 !h-3" />
        </div>
    );
};
