import React, { useCallback, useEffect } from 'react';
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
import {
  LogicalDevice,
  LogicalConnection,
  LogicalDeviceLayout,
  LogicalDeviceRole,
  DEVICE_ROLE_COLORS,
} from './design.model';

// ── Device Node ───────────────────────────────────────────────────────────────

interface DeviceNodeData {
  label: string;
  role: LogicalDeviceRole;
  onSelect?: (deviceId: string) => void;
  deviceId: string;
}

function DeviceNode({ data, selected }: NodeProps<DeviceNodeData>) {
  const colors = DEVICE_ROLE_COLORS[data.role] ?? DEVICE_ROLE_COLORS['Compute'];
  return (
    <div
      style={{
        background: colors.bg,
        border: `2px solid ${selected ? '#154273' : colors.border}`,
        borderRadius: 8,
        padding: '8px 14px',
        fontSize: 12,
        minWidth: 140,
        boxShadow: selected ? '0 0 0 3px rgba(21,66,115,0.15)' : undefined,
        cursor: 'pointer',
      }}
      onClick={() => data.onSelect?.(data.deviceId)}
    >
      <Handle
        type="target"
        position={Position.Top}
        style={{ background: colors.border, width: 8, height: 8 }}
      />
      <div
        style={{
          fontSize: 9,
          fontWeight: 700,
          color: colors.text,
          textTransform: 'uppercase',
          letterSpacing: '0.08em',
          marginBottom: 3,
        }}
      >
        {data.role}
      </div>
      <div style={{ fontWeight: 600, color: '#1e293b' }}>{data.label}</div>
      <Handle
        type="source"
        position={Position.Bottom}
        style={{ background: colors.border, width: 8, height: 8 }}
      />
    </div>
  );
}

const nodeTypes = { device: DeviceNode };

// ── Edge styles by connection type ────────────────────────────────────────────

function edgeStyle(type: string): Partial<Edge> {
  if (type === 'power')
    return {
      style: { stroke: '#f59e0b', strokeWidth: 2, strokeDasharray: '5 3' },
      animated: false,
    };
  if (type === 'console')
    return {
      style: { stroke: '#94a3b8', strokeWidth: 1.5, strokeDasharray: '3 3' },
      animated: false,
    };
  return { style: { stroke: '#154273', strokeWidth: 2 }, animated: true };
}

// ── Props from Angular ────────────────────────────────────────────────────────

export interface DesignFlowProps {
  devices: LogicalDevice[];
  connections: LogicalConnection[];
  layouts: LogicalDeviceLayout[];
  selectedDeviceId: string | null;
  onSelectDevice: (id: string | null) => void;
  onLayoutChange: (layouts: LogicalDeviceLayout[]) => void;
}

// ── Main component ────────────────────────────────────────────────────────────

export function DesignFlow({
  devices,
  connections,
  layouts,
  selectedDeviceId,
  onSelectDevice,
  onLayoutChange,
}: DesignFlowProps) {
  function buildNodes(): Node[] {
    return devices.map((d) => {
      const layout = layouts.find((l) => l.deviceId === d.id);
      return {
        id: d.id,
        type: 'device',
        position: { x: layout?.x ?? 100, y: layout?.y ?? 100 },
        selected: d.id === selectedDeviceId,
        data: {
          label: d.name,
          role: d.role,
          deviceId: d.id,
          onSelect: (id: string) => onSelectDevice(id),
        },
      };
    });
  }

  function buildEdges(): Edge[] {
    return connections.map((c) => ({
      id: c.id,
      source: c.sourceDeviceId,
      target: c.targetDeviceId,
      label: `${c.sourcePortRole} → ${c.targetPortRole}`,
      ...edgeStyle(c.connectionType),
    }));
  }

  const [nodes, setNodes, onNodesChange] = useNodesState(buildNodes());
  const [edges, setEdges, onEdgesChange] = useEdgesState(buildEdges());

  useEffect(() => {
    setNodes(buildNodes());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [devices, layouts, selectedDeviceId]);

  useEffect(() => {
    setEdges(buildEdges());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connections]);

  const onConnect = useCallback(
    (connection: Connection) => setEdges((eds) => addEdge(connection, eds)),
    [setEdges],
  );

  const onNodeDragStop = useCallback(
    (_: unknown, node: Node) => {
      const updated = devices.map((d) => {
        const existing = layouts.find((l) => l.deviceId === d.id);
        if (d.id === node.id) return { deviceId: d.id, x: node.position.x, y: node.position.y };
        return existing ?? { deviceId: d.id, x: 100, y: 100 };
      });
      onLayoutChange(updated);
    },
    [devices, layouts, onLayoutChange],
  );

  const onPaneClick = useCallback(() => {
    onSelectDevice(null);
  }, [onSelectDevice]);

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onConnect={onConnect}
      onNodeDragStop={onNodeDragStop}
      onPaneClick={onPaneClick}
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
