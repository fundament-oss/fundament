import React, { useCallback } from 'react';
import ReactFlow, {
  addEdge,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  Handle,
  Position,
  Connection,
  NodeProps,
  Node,
  Edge,
  ConnectionMode,
} from 'reactflow';

// --- Custom Device Node ---

interface DeviceNodeData {
  label: string;
  type: 'server' | 'switch' | 'patch-panel';
}

const PORT_COUNT = 3;

const deviceStyle: React.CSSProperties = {
  background: '#ffffff',
  border: '1px solid #94a3b8',
  borderRadius: 4,
  padding: '6px 16px',
  fontSize: 11,
  fontFamily: 'monospace',
  width: 180,
  position: 'relative',
};

const labelRowStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 6,
};

const typeIconStyle: React.CSSProperties = {
  fontSize: 10,
  color: '#64748b',
  background: '#f1f5f9',
  borderRadius: 2,
  padding: '1px 4px',
};

function DeviceNode({ data }: NodeProps<DeviceNodeData>) {
  const handleStyle = (index: number, total: number): React.CSSProperties => ({
    top: `${((index + 1) / (total + 1)) * 100}%`,
    width: 8,
    height: 8,
    background: '#94a3b8',
    border: '1px solid #64748b',
    borderRadius: 2,
  });

  return (
    <div style={deviceStyle}>
      {Array.from({ length: PORT_COUNT }, (_, i) => (
        <Handle
          key={`left-${i}`}
          type="source"
          position={Position.Left}
          id={`left-${i}`}
          style={handleStyle(i, PORT_COUNT)}
        />
      ))}

      <div style={labelRowStyle}>
        <span style={typeIconStyle}>
          {data.type === 'server' ? 'SRV' : data.type === 'switch' ? 'SW' : 'PP'}
        </span>
        <span>{data.label}</span>
      </div>

      {Array.from({ length: PORT_COUNT }, (_, i) => (
        <Handle
          key={`right-${i}`}
          type="source"
          position={Position.Right}
          id={`right-${i}`}
          style={handleStyle(i, PORT_COUNT)}
        />
      ))}
    </div>
  );
}

// --- Rack Node ---

function RackNode({ data }: NodeProps<{ label: string }>) {
  return (
    <div
      style={{
        width: '100%',
        height: '100%',
        background: 'rgba(226, 232, 240, 0.4)',
        border: '2px dashed #94a3b8',
        borderRadius: 6,
        pointerEvents: 'none',
      }}
    >
      <div
        style={{
          position: 'absolute',
          top: 8,
          left: 12,
          fontSize: 11,
          fontFamily: 'monospace',
          fontWeight: 600,
          color: '#475569',
          letterSpacing: '0.05em',
          textTransform: 'uppercase',
          pointerEvents: 'none',
        }}
      >
        {data.label}
      </div>
    </div>
  );
}

// --- Node Types ---

const nodeTypes = { device: DeviceNode, rack: RackNode };

// --- Initial Nodes ---

const DEVICE_WIDTH = 180;
const RACK_WIDTH = DEVICE_WIDTH + 40; // 220

const initialNodes: Node[] = [
  // Left rack group
  {
    id: 'rack-left',
    type: 'rack',
    position: { x: 50, y: 50 },
    style: { width: RACK_WIDTH, height: 500 },
    data: { label: 'Rack 1' },
  },
  // Left rack children
  {
    id: 'server-l1',
    type: 'device',
    parentNode: 'rack-left',
    extent: 'parent' as const,
    position: { x: 20, y: 60 },
    data: { label: 'Server L1', type: 'server' },
  },
  {
    id: 'server-l2',
    type: 'device',
    parentNode: 'rack-left',
    extent: 'parent' as const,
    position: { x: 20, y: 160 },
    data: { label: 'Server L2', type: 'server' },
  },
  {
    id: 'switch-l',
    type: 'device',
    parentNode: 'rack-left',
    extent: 'parent' as const,
    position: { x: 20, y: 270 },
    data: { label: 'Switch L', type: 'switch' },
  },
  {
    id: 'pp-l',
    type: 'device',
    parentNode: 'rack-left',
    extent: 'parent' as const,
    position: { x: 20, y: 390 },
    data: { label: 'Patch Panel L', type: 'patch-panel' },
  },

  // Right rack group
  {
    id: 'rack-right',
    type: 'rack',
    position: { x: 420, y: 50 },
    style: { width: RACK_WIDTH, height: 500 },
    data: { label: 'Rack 2' },
  },
  // Right rack children
  {
    id: 'server-r1',
    type: 'device',
    parentNode: 'rack-right',
    extent: 'parent' as const,
    position: { x: 20, y: 60 },
    data: { label: 'Server R1', type: 'server' },
  },
  {
    id: 'server-r2',
    type: 'device',
    parentNode: 'rack-right',
    extent: 'parent' as const,
    position: { x: 20, y: 160 },
    data: { label: 'Server R2', type: 'server' },
  },
  {
    id: 'switch-r',
    type: 'device',
    parentNode: 'rack-right',
    extent: 'parent' as const,
    position: { x: 20, y: 270 },
    data: { label: 'Switch R', type: 'switch' },
  },
  {
    id: 'pp-r',
    type: 'device',
    parentNode: 'rack-right',
    extent: 'parent' as const,
    position: { x: 20, y: 390 },
    data: { label: 'Patch Panel R', type: 'patch-panel' },
  },
];

// --- Initial Edges ---

const initialEdges: Edge[] = [
  // Intra-rack left
  {
    id: 'e-sl1-swl',
    source: 'server-l1',
    sourceHandle: 'right-0',
    target: 'switch-l',
    targetHandle: 'left-0',
  },
  {
    id: 'e-sl2-swl',
    source: 'server-l2',
    sourceHandle: 'right-0',
    target: 'switch-l',
    targetHandle: 'left-1',
  },
  {
    id: 'e-swl-ppl',
    source: 'switch-l',
    sourceHandle: 'right-0',
    target: 'pp-l',
    targetHandle: 'left-0',
  },
  // Intra-rack right
  {
    id: 'e-sr1-swr',
    source: 'server-r1',
    sourceHandle: 'left-0',
    target: 'switch-r',
    targetHandle: 'right-0',
  },
  {
    id: 'e-sr2-swr',
    source: 'server-r2',
    sourceHandle: 'left-0',
    target: 'switch-r',
    targetHandle: 'right-1',
  },
  {
    id: 'e-swr-ppr',
    source: 'switch-r',
    sourceHandle: 'left-0',
    target: 'pp-r',
    targetHandle: 'right-0',
  },
  // Cross-rack cables
  {
    id: 'e-ppl-ppr',
    source: 'pp-l',
    sourceHandle: 'right-0',
    target: 'pp-r',
    targetHandle: 'left-0',
  },
  {
    id: 'e-sl1-sr1',
    source: 'server-l1',
    sourceHandle: 'right-1',
    target: 'server-r1',
    targetHandle: 'left-1',
  },
];

// --- Main Component ---

export function PatchMappingFlow() {
  const [nodes, , onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  const onConnect = useCallback(
    (connection: Connection) => setEdges((eds) => addEdge(connection, eds)),
    [setEdges],
  );

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onConnect={onConnect}
      nodeTypes={nodeTypes}
      connectionMode={ConnectionMode.Loose}
      fitView
      proOptions={{ hideAttribution: true }}
    >
      <Background color="#e2e8f0" gap={20} />
      <Controls />
      <MiniMap nodeColor="#94a3b8" maskColor="rgba(241,245,249,0.7)" />
    </ReactFlow>
  );
}
