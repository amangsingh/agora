export interface Blueprint {
    project: string;
    version: string;
    graph: GraphConfig;
    nodes: NodeConfig[];
    edges: EdgeConfig[];
}

export interface GraphConfig {
    entry: string;
    max_steps: number;
}

export interface NodeConfig {
    name: string;
    type: 'agent' | 'tool' | 'subgraph';
    model?: string;
    instructions?: string;
    tools?: string[];
}

export interface EdgeConfig {
    from: string;
    to: string;
}
