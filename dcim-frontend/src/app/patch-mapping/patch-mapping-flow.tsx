import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
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
import { Cable, CableStatus, CableType, CABLE_COLOR_HEX, Port } from './cable.model';

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
  onEditPorts: (deviceId: string) => void;
  onCableStatusChange: (cableId: string, status: CableStatus) => void;
}

// ── Device node ───────────────────────────────────────────────────────────────

interface DeviceNodeData {
  label: string;
  deviceType: 'server' | 'switch' | 'patch' | 'pdu';
  ports: Port[];
  nodeId: string;
  usedPortIds: Set<string>;
  pendingSourcePortId: string | null;
  onDeviceClick: (id: string) => void;
  onHover: (id: string | null) => void;
  onEditPorts: (deviceId: string) => void;
  onPortClick: (deviceId: string, portId: string, isFree: boolean) => void;
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

function WrenchIcon() {
  return (
    <svg
      width="11"
      height="11"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z" />
    </svg>
  );
}

function DeviceNode({ data }: NodeProps<DeviceNodeData>) {
  const [hovered, setHovered] = useState(false);
  const [btnHovered, setBtnHovered] = useState(false);

  const leftPorts = data.ports.filter(
    (p) =>
      p.type === 'network-interface' ||
      p.type === 'console-port' ||
      p.type === 'console-server-port',
  );
  const rightPorts = data.ports.filter((p) => p.type === 'power-port' || p.type === 'power-outlet');

  const height = Math.max(64, Math.max(leftPorts.length, rightPorts.length) * 18 + 36);

  const handleTopPct = (index: number, total: number) => `${((index + 1) / (total + 1)) * 100}%`;

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
      {/* Left port rows (network/console) */}
      {leftPorts.map((p, i) => {
        const isPending = data.pendingSourcePortId === p.id;
        const isFree = !data.usedPortIds.has(p.id);
        return (
          <div
            key={p.id}
            style={{
              position: 'absolute',
              left: 0,
              top: `calc(${handleTopPct(i, leftPorts.length)} - 9px)`,
              width: '50%',
              height: 18,
              display: 'flex',
              alignItems: 'center',
            }}
          >
            <Handle
              type="source"
              position={Position.Left}
              id={p.id}
              title={`${p.name}${p.label ? ` — ${p.label}` : ''}`}
              style={{
                position: 'absolute',
                left: 0,
                top: 0,
                width: '100%',
                height: '100%',
                opacity: 0,
                borderRadius: 0,
                cursor: isFree ? 'crosshair' : 'default',
                zIndex: 2,
              }}
              onClick={(e) => {
                e.stopPropagation();
                data.onPortClick(data.nodeId, p.id, isFree);
              }}
            />
            {isPending && (
              <span
                style={{
                  position: 'absolute',
                  left: -6,
                  top: 5,
                  width: 8,
                  height: 8,
                  borderRadius: 2,
                  background: '#6366f1',
                  boxShadow: '0 0 0 2px #a5b4fc',
                  animation: 'pulseDot 1s ease-in-out infinite',
                  zIndex: 3,
                }}
              />
            )}
            {!isPending && (
              <span
                style={{
                  position: 'absolute',
                  left: -4,
                  top: 5,
                  width: 8,
                  height: 8,
                  background: isFree ? '#94a3b8' : '#60a5fa',
                  border: '1px solid #64748b',
                  borderRadius: 2,
                  zIndex: 1,
                }}
              />
            )}
          </div>
        );
      })}

      {/* Header row */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
        <span
          style={{
            fontSize: 9,
            color: '#475569',
            background: TYPE_COLOR[data.deviceType] ?? '#f1f5f9',
            borderRadius: 2,
            padding: '1px 4px',
            fontWeight: 600,
            flexShrink: 0,
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
            flex: 1,
          }}
        >
          {data.label}
        </span>
        {/* Wrench button — visible on card hover */}
        {hovered && (
          <button
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              width: 18,
              height: 18,
              borderRadius: 3,
              border: 'none',
              background: btnHovered ? '#e2e8f0' : 'transparent',
              color: btnHovered ? '#334155' : '#94a3b8',
              cursor: 'pointer',
              padding: 0,
              flexShrink: 0,
              transition: 'background 0.1s, color 0.1s',
            }}
            title="Edit ports"
            aria-label={`Edit ports for ${data.label}`}
            onClick={(e) => {
              e.stopPropagation();
              data.onEditPorts(data.nodeId);
            }}
            onMouseEnter={() => setBtnHovered(true)}
            onMouseLeave={() => setBtnHovered(false)}
          >
            <WrenchIcon />
          </button>
        )}
      </div>

      {leftPorts.length > 0 && (
        <div style={{ marginTop: 4, fontSize: 9, color: '#94a3b8' }}>
          {leftPorts.map((p) => p.name).join(' · ')}
        </div>
      )}

      {/* Right port rows (power) */}
      {rightPorts.map((p, i) => {
        const isPending = data.pendingSourcePortId === p.id;
        const isFree = !data.usedPortIds.has(p.id);
        return (
          <div
            key={p.id}
            style={{
              position: 'absolute',
              right: 0,
              top: `calc(${handleTopPct(i, rightPorts.length)} - 9px)`,
              width: '50%',
              height: 18,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'flex-end',
            }}
          >
            <Handle
              type="source"
              position={Position.Right}
              id={p.id}
              title={`${p.name}${p.label ? ` — ${p.label}` : ''}`}
              style={{
                position: 'absolute',
                right: 0,
                top: 0,
                width: '100%',
                height: '100%',
                opacity: 0,
                borderRadius: 0,
                cursor: isFree ? 'crosshair' : 'default',
                zIndex: 2,
              }}
              onClick={(e) => {
                e.stopPropagation();
                data.onPortClick(data.nodeId, p.id, isFree);
              }}
            />
            {isPending && (
              <span
                style={{
                  position: 'absolute',
                  right: -6,
                  top: 5,
                  width: 8,
                  height: 8,
                  borderRadius: 2,
                  background: '#6366f1',
                  boxShadow: '0 0 0 2px #a5b4fc',
                  animation: 'pulseDot 1s ease-in-out infinite',
                  zIndex: 3,
                }}
              />
            )}
            {!isPending && (
              <span
                style={{
                  position: 'absolute',
                  right: -4,
                  top: 5,
                  width: 8,
                  height: 8,
                  background: isFree ? '#94a3b8' : '#60a5fa',
                  border: '1px solid #64748b',
                  borderRadius: 2,
                  zIndex: 1,
                }}
              />
            )}
          </div>
        );
      })}
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
  usedPortIds: Set<string>,
  pendingSourcePortId: string | null,
  onDeviceClick: (id: string) => void,
  onHover: (id: string | null) => void,
  onEditPorts: (deviceId: string) => void,
  onPortClick: (deviceId: string, portId: string, isFree: boolean) => void,
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
      usedPortIds,
      pendingSourcePortId,
      onDeviceClick,
      onHover,
      onEditPorts,
      onPortClick,
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

// ── Cable status context menu ─────────────────────────────────────────────────

const CABLE_STATUSES: { value: CableStatus; label: string }[] = [
  { value: 'connected', label: 'Connected' },
  { value: 'planned', label: 'Planned' },
  { value: 'decommissioned', label: 'Decommissioned' },
];

interface ContextMenu {
  x: number;
  y: number;
  cableId: string;
  currentStatus: CableStatus;
}

function CableContextMenu({
  menu,
  onSelect,
  onClose,
}: {
  menu: ContextMenu;
  onSelect: (status: CableStatus) => void;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as HTMLElement)) onClose();
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [onClose]);

  return (
    <div
      ref={ref}
      style={{
        position: 'fixed',
        top: menu.y,
        left: menu.x,
        zIndex: 9999,
        background: '#ffffff',
        border: '1px solid #e2e8f0',
        borderRadius: 6,
        boxShadow: '0 4px 16px rgba(0,0,0,0.12)',
        padding: '4px 0',
        minWidth: 160,
        fontFamily: 'sans-serif',
        fontSize: 12,
      }}
      role="menu"
      aria-label="Set cable status"
    >
      <div
        style={{
          padding: '4px 12px 6px',
          fontSize: 10,
          fontWeight: 700,
          color: '#94a3b8',
          textTransform: 'uppercase',
          letterSpacing: '0.05em',
        }}
      >
        Set status
      </div>
      {CABLE_STATUSES.map((s) => (
        <button
          key={s.value}
          role="menuitem"
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            width: '100%',
            padding: '5px 12px',
            background: 'transparent',
            border: 'none',
            cursor: 'pointer',
            textAlign: 'left',
            color: s.value === menu.currentStatus ? '#6366f1' : '#334155',
            fontWeight: s.value === menu.currentStatus ? 600 : 400,
            fontSize: 12,
            fontFamily: 'sans-serif',
          }}
          onMouseEnter={(e) =>
            ((e.currentTarget as HTMLButtonElement).style.background = '#f1f5f9')
          }
          onMouseLeave={(e) =>
            ((e.currentTarget as HTMLButtonElement).style.background = 'transparent')
          }
          onClick={() => onSelect(s.value)}
        >
          {s.value === menu.currentStatus && (
            <svg
              width="12"
              height="12"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="3"
            >
              <polyline points="20 6 9 17 4 12" />
            </svg>
          )}
          {s.value !== menu.currentStatus && <span style={{ width: 12 }} />}
          {s.label}
        </button>
      ))}
    </div>
  );
}

// ── Main component ────────────────────────────────────────────────────────────

// Keyframe injected once for the pending port pulse animation
const PULSE_STYLE_ID = 'pm-pulse-keyframe';
if (typeof document !== 'undefined' && !document.getElementById(PULSE_STYLE_ID)) {
  const style = document.createElement('style');
  style.id = PULSE_STYLE_ID;
  style.textContent = `@keyframes pulseDot { 0%,100% { box-shadow: 0 0 0 2px #a5b4fc; } 50% { box-shadow: 0 0 0 4px #c7d2fe; } }`;
  document.head.appendChild(style);
}

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
  onEditPorts,
  onCableStatusChange,
}: PatchMappingFlowProps) {
  const [hoveredDeviceId, setHoveredDeviceId] = useState<string | null>(null);
  const [pendingSource, setPendingSource] = useState<{ deviceId: string; portId: string } | null>(
    null,
  );
  const [contextMenu, setContextMenu] = useState<ContextMenu | null>(null);

  const filteredCables = useMemo(
    () =>
      cables.filter((c) => {
        if (filterStatus && c.status !== filterStatus) return false;
        if (filterType && c.type !== filterType) return false;
        return true;
      }),
    [cables, filterStatus, filterType],
  );

  const usedPortIds = useMemo(
    () => new Set(cables.flatMap((c) => [c.aSide.portId, c.bSide.portId])),
    [cables],
  );

  const onPortClick = useCallback(
    (deviceId: string, portId: string, isFree: boolean) => {
      if (!isFree) {
        // Highlight the cable that uses this port
        const cable = cables.find((c) => c.aSide.portId === portId || c.bSide.portId === portId);
        if (cable) onCableClick(cable.id);
        return;
      }
      if (!pendingSource) {
        setPendingSource({ deviceId, portId });
        return;
      }
      if (pendingSource.deviceId === deviceId) {
        // Same device — cancel
        setPendingSource(null);
        return;
      }
      onConnectionMade({
        sourceDeviceId: pendingSource.deviceId,
        sourcePortId: pendingSource.portId,
        targetDeviceId: deviceId,
        targetPortId: portId,
      });
      setPendingSource(null);
    },
    [pendingSource, cables, onCableClick, onConnectionMade],
  );

  const nodes = useMemo(
    () =>
      buildNodes(
        dcId,
        devicePorts,
        usedPortIds,
        pendingSource?.portId ?? null,
        onDeviceClick,
        setHoveredDeviceId,
        onEditPorts,
        onPortClick,
      ),
    [dcId, devicePorts, usedPortIds, pendingSource, onDeviceClick, onEditPorts, onPortClick],
  );

  const [, , onNodesChange] = useNodesState(nodes);

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

  // ESC cancels pending connection mode
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setPendingSource(null);
        setContextMenu(null);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, []);

  const onEdgeClick = useCallback(
    (_: React.MouseEvent, edge: Edge) => {
      setPendingSource(null);
      onCableClick(edge.id);
    },
    [onCableClick],
  );

  const onEdgeContextMenu = useCallback(
    (e: React.MouseEvent, edge: Edge) => {
      e.preventDefault();
      setPendingSource(null);
      const cable = cables.find((c) => c.id === edge.id);
      if (!cable) return;
      setContextMenu({ x: e.clientX, y: e.clientY, cableId: edge.id, currentStatus: cable.status });
    },
    [cables],
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
    <>
      {/* Pulse keyframe style is injected globally above */}
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onEdgeClick={onEdgeClick}
        onEdgeContextMenu={onEdgeContextMenu}
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
        {pendingSource && (
          <Panel position="top-center">
            <div
              style={{
                background: '#eef2ff',
                border: '1px solid #a5b4fc',
                borderRadius: 6,
                padding: '6px 14px',
                fontSize: 12,
                fontFamily: 'sans-serif',
                color: '#3730a3',
                display: 'flex',
                alignItems: 'center',
                gap: 8,
                boxShadow: '0 2px 8px rgba(99,102,241,0.15)',
              }}
              role="status"
              aria-live="polite"
            >
              <span>Click a free port on another device to complete the cable</span>
              <button
                style={{
                  background: 'transparent',
                  border: 'none',
                  color: '#6366f1',
                  cursor: 'pointer',
                  fontSize: 11,
                  padding: '0 4px',
                  borderRadius: 3,
                  fontFamily: 'sans-serif',
                }}
                onClick={() => setPendingSource(null)}
                aria-label="Cancel patch-from-here mode"
              >
                Esc
              </button>
            </div>
          </Panel>
        )}
      </ReactFlow>
      {contextMenu && (
        <CableContextMenu
          menu={contextMenu}
          onSelect={(status) => {
            onCableStatusChange(contextMenu.cableId, status);
            setContextMenu(null);
          }}
          onClose={() => setContextMenu(null)}
        />
      )}
    </>
  );
}
