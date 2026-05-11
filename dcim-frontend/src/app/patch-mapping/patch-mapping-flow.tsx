import React, { useCallback, useMemo } from 'react';
import ReactFlow, {
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  Handle,
  Position,
  ConnectionMode,
  NodeProps,
  Node,
  Edge,
} from 'reactflow';
import { Cable, CABLE_COLOR_HEX, Port } from './cable.model';

// ── Props ─────────────────────────────────────────────────────────────────────

interface PatchMappingFlowProps {
  cables: Cable[];
  devicePorts: Record<string, Port[]>;
  selectedCableId: string | null;
  onCableClick: (id: string) => void;
  onDeviceClick: (id: string) => void;
}

// ── Device node ───────────────────────────────────────────────────────────────

interface DeviceNodeData {
  label: string;
  deviceType: 'server' | 'switch' | 'patch' | 'pdu';
  ports: Port[];
  nodeId: string;
  onDeviceClick: (id: string) => void;
}

const TYPE_BADGE: Record<string, string> = {
  server: 'SRV',
  switch: 'SW',
  patch: 'PP',
  pdu: 'PDU',
};

const TYPE_COLOR: Record<string, string> = {
  server: '#dbeafe',
  switch: '#d1fae5',
  patch: '#fef3c7',
  pdu: '#ede9fe',
};

function DeviceNode({ data }: NodeProps<DeviceNodeData>) {
  const leftPorts = data.ports.filter(
    (p) =>
      p.type === 'network-interface' ||
      p.type === 'console-port' ||
      p.type === 'console-server-port',
  );
  const rightPorts = data.ports.filter((p) => p.type === 'power-port' || p.type === 'power-outlet');

  const height = Math.max(64, Math.max(leftPorts.length, rightPorts.length) * 18 + 36);

  const handleStyle = (index: number, total: number): React.CSSProperties => ({
    top: `${((index + 1) / (total + 1)) * 100}%`,
    width: 8,
    height: 8,
    background: '#94a3b8',
    border: '1px solid #64748b',
    borderRadius: 2,
  });

  return (
    <div
      style={{
        background: '#ffffff',
        border: '1px solid #94a3b8',
        borderRadius: 4,
        padding: '6px 14px',
        fontSize: 11,
        fontFamily: 'monospace',
        width: 190,
        minHeight: height,
        position: 'relative',
        cursor: 'pointer',
        boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
      }}
      onClick={() => data.onDeviceClick(data.nodeId)}
    >
      {leftPorts.map((p, i) => (
        <Handle
          key={p.id}
          type="source"
          position={Position.Left}
          id={p.id}
          title={p.name}
          style={handleStyle(i, leftPorts.length)}
        />
      ))}

      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
        <span
          style={{
            fontSize: 9,
            color: '#475569',
            background: TYPE_COLOR[data.deviceType] ?? '#f1f5f9',
            borderRadius: 2,
            padding: '1px 4px',
            fontWeight: 600,
          }}
        >
          {TYPE_BADGE[data.deviceType] ?? data.deviceType.toUpperCase()}
        </span>
        <span
          style={{
            color: '#1e293b',
            fontWeight: 600,
            overflow: 'hidden',
            whiteSpace: 'nowrap',
            textOverflow: 'ellipsis',
          }}
        >
          {data.label}
        </span>
      </div>

      {leftPorts.length > 0 && (
        <div style={{ marginTop: 4, fontSize: 9, color: '#94a3b8' }}>
          {leftPorts.map((p) => p.name).join(' · ')}
        </div>
      )}

      {rightPorts.map((p, i) => (
        <Handle
          key={p.id}
          type="source"
          position={Position.Right}
          id={p.id}
          title={p.name}
          style={handleStyle(i, rightPorts.length)}
        />
      ))}
    </div>
  );
}

// ── Rack container node ───────────────────────────────────────────────────────

function RackNode({ data }: NodeProps<{ label: string }>) {
  return (
    <div
      style={{
        width: '100%',
        height: '100%',
        background: 'rgba(226, 232, 240, 0.35)',
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
          fontWeight: 700,
          color: '#475569',
          letterSpacing: '0.06em',
          textTransform: 'uppercase',
          pointerEvents: 'none',
        }}
      >
        {data.label}
      </div>
    </div>
  );
}

const nodeTypes = { device: DeviceNode, rack: RackNode };

// ── Static layout for AMS-01 devices ─────────────────────────────────────────
// Device IDs match rack.model.ts. Layout is position-only; ports come from props.

interface DeviceLayout {
  id: string;
  label: string;
  deviceType: 'server' | 'switch' | 'patch' | 'pdu';
  parentNode: string;
  position: { x: number; y: number };
}

const RACK_LAYOUTS: {
  id: string;
  label: string;
  x: number;
  y: number;
  width: number;
  height: number;
}[] = [
  { id: 'rack-r01', label: 'AMS-01-R01', x: 40, y: 40, width: 230, height: 480 },
  { id: 'rack-r02', label: 'AMS-01-R02', x: 320, y: 40, width: 230, height: 360 },
  { id: 'rack-r04', label: 'AMS-01-R04', x: 600, y: 40, width: 230, height: 200 },
];

const DEVICE_LAYOUTS: DeviceLayout[] = [
  // R01
  {
    id: 'd-001',
    label: 'tor-switch-01',
    deviceType: 'switch',
    parentNode: 'rack-r01',
    position: { x: 20, y: 50 },
  },
  {
    id: 'd-002',
    label: 'patch-panel-01',
    deviceType: 'patch',
    parentNode: 'rack-r01',
    position: { x: 20, y: 155 },
  },
  {
    id: 'd-003',
    label: 'server-01',
    deviceType: 'server',
    parentNode: 'rack-r01',
    position: { x: 20, y: 260 },
  },
  {
    id: 'd-008',
    label: 'pdu-01',
    deviceType: 'pdu',
    parentNode: 'rack-r01',
    position: { x: 20, y: 390 },
  },
  // R02
  {
    id: 'd-101',
    label: 'leaf-switch-01',
    deviceType: 'switch',
    parentNode: 'rack-r02',
    position: { x: 20, y: 50 },
  },
  {
    id: 'd-102',
    label: 'server-10',
    deviceType: 'server',
    parentNode: 'rack-r02',
    position: { x: 20, y: 180 },
  },
  {
    id: 'd-103',
    label: 'server-11',
    deviceType: 'server',
    parentNode: 'rack-r02',
    position: { x: 20, y: 290 },
  },
  // R04
  {
    id: 'd-301',
    label: 'spine-switch-01',
    deviceType: 'switch',
    parentNode: 'rack-r04',
    position: { x: 20, y: 50 },
  },
];

function buildNodes(
  devicePorts: Record<string, Port[]>,
  onDeviceClick: (id: string) => void,
): Node[] {
  const rackNodes: Node[] = RACK_LAYOUTS.map((r) => ({
    id: r.id,
    type: 'rack',
    position: { x: r.x, y: r.y },
    style: { width: r.width, height: r.height },
    data: { label: r.label },
    selectable: false,
    draggable: false,
  }));

  const deviceNodes: Node[] = DEVICE_LAYOUTS.map((d) => ({
    id: d.id,
    type: 'device',
    parentNode: d.parentNode,
    extent: 'parent' as const,
    position: d.position,
    data: {
      label: d.label,
      deviceType: d.deviceType,
      ports: devicePorts[d.id] ?? [],
      nodeId: d.id,
      onDeviceClick,
    },
  }));

  return [...rackNodes, ...deviceNodes];
}

// ── Main component ────────────────────────────────────────────────────────────

export function PatchMappingFlow({
  cables,
  devicePorts,
  selectedCableId,
  onCableClick,
  onDeviceClick,
}: PatchMappingFlowProps) {
  const initialNodes = useMemo(
    () => buildNodes(devicePorts, onDeviceClick),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  const [nodes, , onNodesChange] = useNodesState(initialNodes);

  const edges = useMemo<Edge[]>(
    () =>
      cables.map((cable) => ({
        id: cable.id,
        source: cable.aSide.deviceId,
        sourceHandle: cable.aSide.portId,
        target: cable.bSide.deviceId,
        targetHandle: cable.bSide.portId,
        label: cable.label,
        selected: cable.id === selectedCableId,
        style: {
          stroke: cable.color ? CABLE_COLOR_HEX[cable.color] : '#94a3b8',
          strokeWidth: cable.id === selectedCableId ? 3 : 1.5,
          strokeDasharray: cable.status === 'planned' ? '6 3' : undefined,
          opacity: cable.status === 'decommissioned' ? 0.3 : 1,
        },
        labelStyle: { fontSize: 10, fill: '#475569' },
        labelBgStyle: { fill: '#f8fafc', fillOpacity: 0.9 },
      })),
    [cables, selectedCableId],
  );

  const [, setEdges, onEdgesChange] = useEdgesState(edges);

  // Keep edges in sync with prop changes
  React.useEffect(() => {
    setEdges(edges);
  }, [edges, setEdges]);

  const onEdgeClick = useCallback(
    (_: React.MouseEvent, edge: Edge) => {
      onCableClick(edge.id);
    },
    [onCableClick],
  );

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onEdgeClick={onEdgeClick}
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
