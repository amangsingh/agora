import { create } from 'zustand';
import {
    addEdge,
    applyNodeChanges,
    applyEdgeChanges,
    type Connection,
    type Edge,
    type EdgeChange,
    type Node as RFNode,
    type NodeChange,
    type OnNodesChange,
    type OnEdgesChange,
    type OnConnect,
} from '@xyflow/react';
import type { Blueprint } from '../types/blueprint';

// Explicitly define what data our nodes carry
export type NodeData = {
    label: string;
    config?: any;
};

interface GraphState {
    nodes: RFNode<NodeData>[]; // Typed nodes
    edges: Edge[];
    onNodesChange: OnNodesChange<RFNode<NodeData>>;
    onEdgesChange: OnEdgesChange;
    onConnect: OnConnect;
    addNode: (type: 'agent' | 'tool') => void;
    syncToBlueprint: () => Blueprint;
}

export const useGraphStore = create<GraphState>((set, get) => ({
    nodes: [],
    edges: [],

    onNodesChange: (changes: NodeChange<RFNode<NodeData>>[]) => {
        set({
            nodes: applyNodeChanges<RFNode<NodeData>>(changes, get().nodes),
        });
    },

    onEdgesChange: (changes: EdgeChange[]) => {
        set({
            edges: applyEdgeChanges(changes, get().edges),
        });
    },

    onConnect: (connection: Connection) => {
        set({
            edges: addEdge(connection, get().edges),
        });
    },

    addNode: (type: 'agent' | 'tool') => {
        const id = Math.random().toString(36).substring(7);
        const newNode: RFNode<NodeData> = {
            id,
            type,
            position: { x: window.innerWidth / 2, y: window.innerHeight / 2 },
            data: { label: `${type}_${id}` },
        };
        set({ nodes: [...get().nodes, newNode] });
    },

    syncToBlueprint: () => {
        const { nodes, edges } = get();

        // Map ReactFlow Nodes to Blueprint Nodes
        const bpNodes = nodes.map(n => ({
            name: n.data.label,
            type: n.type as 'agent' | 'tool' | 'subgraph',
            // Defaults for now
            model: n.type === 'agent' ? 'llama3' : undefined,
            instructions: n.type === 'agent' ? 'Output system prompt' : undefined,
        }));

        // Map ReactFlow Edges to Blueprint Edges
        const bpEdges = edges.map(e => {
            // Safe access to node data
            const sourceNode = get().nodes.find(n => n.id === e.source);
            const targetNode = get().nodes.find(n => n.id === e.target);
            return {
                from: sourceNode?.data.label || e.source,
                to: targetNode?.data.label || e.target,
            };
        });

        // Determine Entry Point
        const entry = bpNodes.find(n => n.type === 'agent')?.name || "";

        return {
            project: "my-agent",
            version: "1.0.0",
            graph: {
                entry,
                max_steps: 25,
            },
            nodes: bpNodes,
            edges: bpEdges,
        };
    },
}));
