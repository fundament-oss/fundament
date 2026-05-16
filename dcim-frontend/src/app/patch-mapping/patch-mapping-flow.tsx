import React, { useCallback, useMemo, useState } from 'react';
import ReactFlow, {
  Background,
  Controls,
  MiniMap,
  Panel,
  useNodesState,
  useEdgesState,
  Handle,
  Position,
  ConnectionMode,
  NodeProps,
  Node,
  Edge,
  Connection,
} from 'reactflow';
import {
  Cable,
  CableStatus,
  CableType,
  CABLE_COLOR_HEX,
  CABLE_TYPE_LABEL,
  Port,
} from './cable.model';

// ── Props ─────────────────────────────────────────────────────────────────────

interface PatchMappingFlowProps {
  cables: Cable[];
  devicePorts: Record<string, Port[]>;
  selectedCableId: string | null;
  dcId: string;
  filterStatus: CableStatus | '';
  filterType: CableType | '';
  onCableClick: (id: string) => void;
  onDeviceClick: (id: string) => void;
  onConnectionMade: (conn: {
    sourceDeviceId: string;
    sourcePortId: string;
    targetDeviceId: string;
    targetPortId: string;
  }) => void;
}

// ── Device node ───────────────────────────────────────────────────────────────

interface DeviceNodeData {
  label: string;
  deviceType: 'server' | 'switch' | 'patch' | 'pdu';
  ports: Port[];
  nodeId: string;
  onDeviceClick: (id: string) => void;
  onHover: (id: string | null) => void;
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
  const [hovered, setHovered] = useState(false);

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
        background: hovered ? '#f8fafc' : '#ffffff',
        border: `1px solid ${hovered ? '#64748b' : '#94a3b8'}`,
        borderRadius: 4,
        padding: '6px 14px',
        fontSize: 11,
        fontFamily: 'monospace',
        width: 190,
        minHeight: height,
        position: 'relative',
        cursor: 'pointer',
        boxShadow: hovered ? '0 2px 8px rgba(0,0,0,0.14)' : '0 1px 3px rgba(0,0,0,0.08)',
        transition: 'box-shadow 0.15s, border-color 0.15s, background 0.15s',
      }}
      onClick={() => data.onDeviceClick(data.nodeId)}
      onMouseEnter={() => {
        setHovered(true);
        data.onHover(data.nodeId);
      }}
      onMouseLeave={() => {
        setHovered(false);
        data.onHover(null);
      }}
    >
      {leftPorts.map((p, i) => (
        <Handle
          key={p.id}
          type="source"
          position={Position.Left}
          id={p.id}
          title={`${p.name}${p.label ? ` — ${p.label}` : ''}`}
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
          title={`${p.name}${p.label ? ` — ${p.label}` : ''}`}
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

// ── Layout data ───────────────────────────────────────────────────────────────

interface RackLayout {
  id: string;
  label: string;
  dcId: string;
  x: number;
  y: number;
  width: number;
  height: number;
}

interface DeviceLayout {
  id: string;
  label: string;
  deviceType: 'server' | 'switch' | 'patch' | 'pdu';
  parentNode: string;
  dcId: string;
  position: { x: number; y: number };
}

const RACK_LAYOUTS: RackLayout[] = [
  // AMS-01
  { id: 'rack-r01', label: 'AMS-01-R01', dcId: 'ams-01', x: 40, y: 40, width: 230, height: 480 },
  { id: 'rack-r02', label: 'AMS-01-R02', dcId: 'ams-01', x: 320, y: 40, width: 230, height: 360 },
  { id: 'rack-r04', label: 'AMS-01-R04', dcId: 'ams-01', x: 600, y: 40, width: 230, height: 200 },
  // FRA-01
  {
    id: 'rack-fra01-r01',
    label: 'FRA-01-R01',
    dcId: 'fra-01',
    x: 40,
    y: 40,
    width: 230,
    height: 320,
  },
];

const DEVICE_LAYOUTS: DeviceLayout[] = [
  // AMS-01 R01
  {
    id: 'd-001',
    label: 'tor-switch-01',
    deviceType: 'switch',
    parentNode: 'rack-r01',
    dcId: 'ams-01',
    position: { x: 20, y: 50 },
  },
  {
    id: 'd-002',
    label: 'patch-panel-01',
    deviceType: 'patch',
    parentNode: 'rack-r01',
    dcId: 'ams-01',
    position: { x: 20, y: 155 },
  },
  {
    id: 'd-003',
    label: 'server-01',
    deviceType: 'server',
    parentNode: 'rack-r01',
    dcId: 'ams-01',
    position: { x: 20, y: 260 },
  },
  {
    id: 'd-008',
    label: 'pdu-01',
    deviceType: 'pdu',
    parentNode: 'rack-r01',
    dcId: 'ams-01',
    position: { x: 20, y: 390 },
  },
  // AMS-01 R02
  {
    id: 'd-101',
    label: 'leaf-switch-01',
    deviceType: 'switch',
    parentNode: 'rack-r02',
    dcId: 'ams-01',
    position: { x: 20, y: 50 },
  },
  {
    id: 'd-102',
    label: 'server-10',
    deviceType: 'server',
    parentNode: 'rack-r02',
    dcId: 'ams-01',
    position: { x: 20, y: 180 },
  },
  {
    id: 'd-103',
    label: 'server-11',
    deviceType: 'server',
    parentNode: 'rack-r02',
    dcId: 'ams-01',
    position: { x: 20, y: 290 },
  },
  // AMS-01 R04
  {
    id: 'd-301',
    label: 'spine-switch-01',
    deviceType: 'switch',
    parentNode: 'rack-r04',
    dcId: 'ams-01',
    position: { x: 20, y: 50 },
  },
  // FRA-01 R01
  {
    id: 'd-601',
    label: 'tor-switch-01',
    deviceType: 'switch',
    parentNode: 'rack-fra01-r01',
    dcId: 'fra-01',
    position: { x: 20, y: 50 },
  },
  {
    id: 'd-603',
    label: 'server-61',
    deviceType: 'server',
    parentNode: 'rack-fra01-r01',
    dcId: 'fra-01',
    position: { x: 20, y: 180 },
  },
];

function buildNodes(
  dcId: string,
  devicePorts: Record<string, Port[]>,
  onDeviceClick: (id: string) => void,
  onHover: (id: string | null) => void,
): Node[] {
  const dcRacks = RACK_LAYOUTS.filter((r) => r.dcId === dcId);
  const dcDevices = DEVICE_LAYOUTS.filter((d) => d.dcId === dcId);

  const rackNodes: Node[] = dcRacks.map((r) => ({
    id: r.id,
    type: 'rack',
    position: { x: r.x, y: r.y },
    style: { width: r.width, height: r.height },
    data: { label: r.label },
    selectable: false,
    draggable: false,
  }));

  const deviceNodes: Node[] = dcDevices.map((d) => ({
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
      onHover,
    },
  }));

  return [...rackNodes, ...deviceNodes];
}

// ── Legend panel ─────────────────────────────────────────────────────────────

function Legend() {
  return (
    <div
      style={{
        background: 'rgba(255,255,255,0.92)',
        border: '1px solid #e2e8f0',
        borderRadius: 6,
        padding: '8px 12px',
        fontSize: 10,
        fontFamily: 'sans-serif',
        color: '#475569',
        display: 'flex',
        flexDirection: 'column',
        gap: 5,
        boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
      }}
    >
      <div
        style={{
          fontWeight: 700,
          marginBottom: 2,
          letterSpacing: '0.04em',
          textTransform: 'uppercase',
          fontSize: 9,
        }}
      >
        Legend
      </div>
      <LegendRow dash={false} opacity={1} label="Connected" />
      <LegendRow dash={true} opacity={1} label="Planned" />
      <LegendRow dash={false} opacity={0.3} label="Decommissioned" />
    </div>
  );
}

function LegendRow({ dash, opacity, label }: { dash: boolean; opacity: number; label: string }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, opacity }}>
      <svg width="28" height="10" style={{ flexShrink: 0 }}>
        <line
          x1="2"
          y1="5"
          x2="26"
          y2="5"
          stroke="#64748b"
          strokeWidth="2"
          strokeDasharray={dash ? '5 3' : undefined}
        />
      </svg>
      <span>{label}</span>
    </div>
  );
}

// ── Main component ────────────────────────────────────────────────────────────

export function PatchMappingFlow({
  cables,
  devicePorts,
  selectedCableId,
  dcId,
  filterStatus,
  filterType,
  onCableClick,
  onDeviceClick,
  onConnectionMade,
}: PatchMappingFlowProps) {
  const [hoveredDeviceId, setHoveredDeviceId] = useState<string | null>(null);

  const nodes = useMemo(
    () => buildNodes(dcId, devicePorts, onDeviceClick, setHoveredDeviceId),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [dcId, devicePorts],
  );

  const [, , onNodesChange] = useNodesState(nodes);

  const filteredCables = useMemo(
    () =>
      cables.filter((c) => {
        if (filterStatus && c.status !== filterStatus) return false;
        if (filterType && c.type !== filterType) return false;
        return true;
      }),
    [cables, filterStatus, filterType],
  );

  const edges = useMemo<Edge[]>(
    () =>
      filteredCables.map((cable) => {
        const isConnected =
          !hoveredDeviceId ||
          cable.aSide.deviceId === hoveredDeviceId ||
          cable.bSide.deviceId === hoveredDeviceId;

        return {
          id: cable.id,
          source: cable.aSide.deviceId,
          sourceHandle: cable.aSide.portId,
          target: cable.bSide.deviceId,
          targetHandle: cable.bSide.portId,
          label: cable.label,
          selected: cable.id === selectedCableId,
          zIndex: 10,
          style: {
            stroke: cable.color ? CABLE_COLOR_HEX[cable.color] : '#94a3b8',
            strokeWidth: cable.id === selectedCableId ? 3 : 1.5,
            strokeDasharray: cable.status === 'planned' ? '6 3' : undefined,
            opacity: cable.status === 'decommissioned' ? 0.3 : isConnected ? 1 : 0.15,
            transition: 'opacity 0.15s',
          },
          labelStyle: { fontSize: 10, fill: '#475569' },
          labelBgStyle: { fill: '#f8fafc', fillOpacity: 0.9 },
        };
      }),
    [filteredCables, selectedCableId, hoveredDeviceId],
  );

  const [, setEdges, onEdgesChange] = useEdgesState(edges);

  React.useEffect(() => {
    setEdges(edges);
  }, [edges, setEdges]);

  const onEdgeClick = useCallback(
    (_: React.MouseEvent, edge: Edge) => {
      onCableClick(edge.id);
    },
    [onCableClick],
  );

  const onConnect = useCallback(
    (connection: Connection) => {
      if (
        connection.source &&
        connection.sourceHandle &&
        connection.target &&
        connection.targetHandle
      ) {
        onConnectionMade({
          sourceDeviceId: connection.source,
          sourcePortId: connection.sourceHandle,
          targetDeviceId: connection.target,
          targetPortId: connection.targetHandle,
        });
      }
    },
    [onConnectionMade],
  );

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onEdgeClick={onEdgeClick}
      onConnect={onConnect}
      nodeTypes={nodeTypes}
      connectionMode={ConnectionMode.Loose}
      fitView
      fitViewOptions={{ padding: 0.15 }}
      proOptions={{ hideAttribution: true }}
    >
      <Background color="#e2e8f0" gap={20} />
      <Controls />
      <MiniMap nodeColor="#94a3b8" maskColor="rgba(241,245,249,0.7)" />
      <Panel position="bottom-left">
        <Legend />
      </Panel>
    </ReactFlow>
  );
}
