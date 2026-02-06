import React from 'react';
import Editor from '@monaco-editor/react';
import { useGraphStore } from '../../stores/graphStore';
import YAML from 'yaml'; // We might need to add this dep or just use JSON for now. Let's use JSON for v1 simplicity or mock YAML.
// Attempting to standard JSON for the editor view to match our Blueprint structs for now.

const BlueprintEditor = () => {
    const { syncToBlueprint } = useGraphStore();
    const [code, setCode] = React.useState('');

    // Poll for changes or use an effect?
    // For specific "Two-Way Sync" we usually need granular updates.
    // For this MVP, we will just render the current blueprint on open/mount or intervals.

    // Better: Effect that runs when store changes? 
    // Ideally we subscribe to store. 
    // But `syncToBlueprint` is a function. 
    // We can use `useGraphStore` to read nodes/edges and derive blueprint.

    // Let's just output JSON for now to prove strict compliance.
    const blueprint = syncToBlueprint();
    const jsonString = JSON.stringify(blueprint, null, 2);

    return (
        <div className="h-full flex flex-col bg-[#1e1e1e]">
            <div className="p-2 bg-[#2d2d2d] text-slate-300 text-xs font-mono uppercase tracking-wider border-b border-black">
                Source Code (Live)
            </div>
            <Editor
                height="100%"
                defaultLanguage="json"
                value={jsonString}
                theme="vs-dark"
                options={{
                    readOnly: true, // For MVP v1, one-way sync Visual -> Code
                    minimap: { enabled: false },
                    fontSize: 12,
                    fontFamily: 'JetBrains Mono, monospace',
                }}
            />
        </div>
    );
};

export default BlueprintEditor;
