import React, { useCallback, useEffect, useRef, useState, useMemo } from 'react';
import ReactFlow, {
  Background,
  Controls,
  MiniMap,
  Panel,
  useEdgesState,
  Handle,
  Position,
  ConnectionMode,
  NodeProps,
  Node,
  Edge,
  Connection,
  EdgeProps,
  BaseEdge,
  getBezierPath,
} from 'reactflow';
import {
  Cable,
  CableStatus,
  CableType,
  CABLE_COLOR_HEX,
  Port,
  PortType,
  portsAreCompatible,
} from './cable.model';

// ── Port type visual styles ───────────────────────────────────────────────────

const PORT_TYPE_STYLE: Record<PortType, { free: string; used: string; abbr: string }> = {
  'network-interface': { free: '#bfdbfe', used: '#2563eb', abbr: 'NET' },
  'console-port': { free: '#fde68a', used: '#d97706', abbr: 'CON' },
  'console-server-port': { free: '#fde68a', used: '#d97706', abbr: 'CON' },
  'power-port': { free: '#fecaca', used: '#dc2626', abbr: 'PWR' },
  'power-outlet': { free: '#e9d5ff', used: '#9333ea', abbr: 'OUT' },
};

const PORT_ROW_H = 22;
const PORT_SQ = 12;
const PORT_INSET = 6; // distance outside card edge for port square center

// ── Props ─────────────────────────────────────────────────────────────────────

/** A device (placement) in the active datacenter, used to build topology nodes. */
export interface DeviceInfo {
  id: string;
  name: string;
}

interface PatchMappingFlowProps {
  cables: Cable[];
  devicePorts: Record<string, Port[]>;
  devices: DeviceInfo[];
  selectedCableId: string | null;
  dcId: string;
  filterStatus: CableStatus | '';
  filterType: CableType | '';
  onCableClick: (id: string) => void;
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
  activeSourceType: PortType | null;
  onBodyClick: () => void;
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

function PortRow({
  port,
  index,
  total,
  side,
  isFree,
  isPending,
  isHovered,
  isIncompatible,
  deviceId,
  onPortClick,
  onHoverPort,
}: {
  port: Port;
  index: number;
  total: number;
  side: 'left' | 'right';
  isFree: boolean;
  isPending: boolean;
  isHovered: boolean;
  isIncompatible: boolean;
  deviceId: string;
  onPortClick: (deviceId: string, portId: string, isFree: boolean) => void;
  onHoverPort: (portId: string | null) => void;
}) {
  const mouseDownPos = useRef<{ x: number; y: number } | null>(null);
  const topPct = `${((index + 1) / (total + 1)) * 100}%`;
  const style = PORT_TYPE_STYLE[port.type];
  const squareColor = isPending ? '#6366f1' : isFree ? style.free : style.used;
  const borderColor = isPending ? '#a5b4fc' : isFree ? '#94a3b8' : squareColor;

  const rowStyle: React.CSSProperties = {
    position: 'absolute',
    top: `calc(${topPct} - ${PORT_ROW_H / 2}px)`,
    width: '52%',
    height: PORT_ROW_H,
    display: 'flex',
    alignItems: 'center',
    ...(side === 'left' ? { left: 0 } : { right: 0, flexDirection: 'row-reverse' }),
  };

  const labelStyle: React.CSSProperties = {
    fontSize: 9,
    fontFamily: 'monospace',
    color: isHovered ? '#1e293b' : '#64748b',
    overflow: 'hidden',
    whiteSpace: 'nowrap',
    textOverflow: 'ellipsis',
    maxWidth: 68,
    userSelect: 'none',
    transition: 'color 0.1s',
    ...(side === 'left'
      ? { marginLeft: PORT_INSET + PORT_SQ + 4 }
      : { marginRight: PORT_INSET + PORT_SQ + 4 }),
  };

  return (
    <div
      style={{ ...rowStyle, opacity: isIncompatible ? 0.25 : 1, transition: 'opacity 0.15s' }}
      onMouseEnter={() => onHoverPort(port.id)}
      onMouseLeave={() => onHoverPort(null)}
    >
      {/*
       * The Handle IS the visible port square. Positioning it at exactly the
       * same spot as the old <span> means dragging from the visible square
       * always hits the ReactFlow Handle — eliminating the gap that caused
       * canvas-panning when dragging from the protruding part of the square.
       * `transform: 'none'` suppresses ReactFlow's default translate(-50%).
       */}
      <Handle
        type="source"
        position={side === 'left' ? Position.Left : Position.Right}
        id={port.id}
        title={`${port.name} (${PORT_TYPE_STYLE[port.type].abbr})`}
        isConnectable={isFree && !isIncompatible}
        style={{
          position: 'absolute',
          width: PORT_SQ,
          height: PORT_SQ,
          borderRadius: 2,
          background: squareColor,
          border: `1.5px solid ${borderColor}`,
          boxShadow: isPending ? `0 0 0 3px #c7d2fe` : undefined,
          animation: isPending ? 'pulseDot 1s ease-in-out infinite' : undefined,
          ...(side === 'left' ? { left: -PORT_INSET } : { right: -PORT_INSET }),
          top: `calc(50% - ${PORT_SQ / 2}px)`,
          transform: 'none',
          cursor: !isFree || isIncompatible ? 'not-allowed' : 'crosshair',
          pointerEvents: 'auto',
          zIndex: 2,
        }}
        onMouseDown={(e) => {
          mouseDownPos.current = { x: e.clientX, y: e.clientY };
        }}
        onClick={(e) => {
          e.stopPropagation();
          if (mouseDownPos.current) {
            const dx = e.clientX - mouseDownPos.current.x;
            const dy = e.clientY - mouseDownPos.current.y;
            if (dx * dx + dy * dy > 25) return;
          }
          onPortClick(deviceId, port.id, isFree);
        }}
      />
      <span style={labelStyle}>{port.name}</span>
    </div>
  );
}

function computeDeviceHeight(ports: Port[]): number {
  const leftPorts = ports.filter(
    (p) =>
      p.type === 'network-interface' ||
      p.type === 'console-port' ||
      p.type === 'console-server-port',
  );
  const rightPorts = ports.filter((p) => p.type === 'power-port' || p.type === 'power-outlet');
  const totalPorts = ports.length;
  const headerH = 28;
  const footerH = leftPorts.length > 0 || rightPorts.length > 0 ? 16 : 0;
  const utilBarH = totalPorts > 0 ? 5 : 0;
  const portRows = Math.max(leftPorts.length, rightPorts.length);
  const bodyH = Math.max(portRows * PORT_ROW_H, portRows > 0 ? PORT_ROW_H : 0);
  return Math.max(64, headerH + bodyH + footerH + utilBarH + 4);
}

function DeviceNode({ data }: NodeProps<DeviceNodeData>) {
  const [hovered, setHovered] = useState(false);
  const [hoveredPortId, setHoveredPortId] = useState<string | null>(null);

  const leftPorts = useMemo(
    () =>
      data.ports.filter(
        (p) =>
          p.type === 'network-interface' ||
          p.type === 'console-port' ||
          p.type === 'console-server-port',
      ),
    [data.ports],
  );
  const rightPorts = useMemo(
    () => data.ports.filter((p) => p.type === 'power-port' || p.type === 'power-outlet'),
    [data.ports],
  );

  const totalPorts = data.ports.length;
  const usedCount = data.ports.filter((p) => data.usedPortIds.has(p.id)).length;
  const utilizationPct = totalPorts > 0 ? (usedCount / totalPorts) * 100 : 0;
  const utilColor =
    utilizationPct === 0 ? '#86efac' : utilizationPct === 100 ? '#fca5a5' : '#fde68a';

  const headerH = 28;
  const footerH = leftPorts.length > 0 || rightPorts.length > 0 ? 16 : 0;
  const utilBarH = totalPorts > 0 ? 5 : 0;
  const totalH = computeDeviceHeight(data.ports);

  return (
    <div
      style={{
        background: hovered ? '#f8fafc' : '#ffffff',
        border: `1px solid ${hovered ? '#64748b' : '#94a3b8'}`,
        borderRadius: 4,
        fontSize: 11,
        fontFamily: 'monospace',
        width: 190,
        height: totalH,
        position: 'relative',
        cursor: 'pointer',
        boxShadow: hovered ? '0 2px 8px rgba(0,0,0,0.14)' : '0 1px 3px rgba(0,0,0,0.08)',
        transition: 'box-shadow 0.15s, border-color 0.15s, background 0.15s',
        overflow: 'visible',
      }}
      onClick={() => data.onBodyClick()}
      onMouseEnter={() => {
        setHovered(true);
        data.onHover(data.nodeId);
      }}
      onMouseLeave={() => {
        setHovered(false);
        setHoveredPortId(null);
        data.onHover(null);
      }}
    >
      {/* Port rows — rendered as absolute overlays on left and right */}
      {leftPorts.map((p, i) => (
        <PortRow
          key={p.id}
          port={p}
          index={i}
          total={leftPorts.length}
          side="left"
          isFree={!data.usedPortIds.has(p.id)}
          isPending={data.pendingSourcePortId === p.id}
          isHovered={hoveredPortId === p.id}
          isIncompatible={
            data.activeSourceType !== null && !portsAreCompatible(data.activeSourceType, p.type)
          }
          deviceId={data.nodeId}
          onPortClick={data.onPortClick}
          onHoverPort={setHoveredPortId}
        />
      ))}
      {rightPorts.map((p, i) => (
        <PortRow
          key={p.id}
          port={p}
          index={i}
          total={rightPorts.length}
          side="right"
          isFree={!data.usedPortIds.has(p.id)}
          isPending={data.pendingSourcePortId === p.id}
          isHovered={hoveredPortId === p.id}
          isIncompatible={
            data.activeSourceType !== null && !portsAreCompatible(data.activeSourceType, p.type)
          }
          deviceId={data.nodeId}
          onPortClick={data.onPortClick}
          onHoverPort={setHoveredPortId}
        />
      ))}

      {/* Header */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          padding: '5px 10px',
          height: headerH,
        }}
      >
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
      </div>

      {/* Port name footers */}
      {(leftPorts.length > 0 || rightPorts.length > 0) && (
        <div
          style={{
            position: 'absolute',
            bottom: utilBarH + 2,
            left: 10,
            right: 10,
            height: footerH,
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'flex-end',
            gap: 4,
          }}
        >
          {leftPorts.length > 0 && (
            <span
              style={{
                fontSize: 8,
                color: '#94a3b8',
                overflow: 'hidden',
                whiteSpace: 'nowrap',
                textOverflow: 'ellipsis',
                flex: 1,
              }}
            >
              {leftPorts.map((p) => p.name).join(' · ')}
            </span>
          )}
          {rightPorts.length > 0 && (
            <span
              style={{
                fontSize: 8,
                color: '#94a3b8',
                overflow: 'hidden',
                whiteSpace: 'nowrap',
                textOverflow: 'ellipsis',
                textAlign: 'right',
                flex: 1,
              }}
            >
              {rightPorts.map((p) => p.name).join(' · ')}
            </span>
          )}
        </div>
      )}

      {/* Utilization bar */}
      {totalPorts > 0 && (
        <div
          style={{
            position: 'absolute',
            bottom: 0,
            left: 0,
            right: 0,
            height: utilBarH,
            background: '#e2e8f0',
            borderRadius: '0 0 4px 4px',
            overflow: 'hidden',
          }}
          title={`${usedCount} / ${totalPorts} ports in use`}
        >
          <div
            style={{
              height: '100%',
              width: `${utilizationPct}%`,
              background: utilColor,
              transition: 'width 0.3s, background 0.3s',
            }}
          />
        </div>
      )}
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

// ── Custom edge with adaptive curvature ───────────────────────────────────────
// getBezierPath computes control-point offsets from |dx|. When both ports are on
// the same side (dx ≈ 0) the resulting path is nearly a vertical line and hard to
// distinguish from rack-column borders. For same-side connections we build the
// cubic bezier manually, forcing the control points to bow out by a fixed amount.

const SAME_SIDE_BOW = 90; // px the curve bulges outward for same-side cables

function buildSameSidePath(
  sx: number,
  sy: number,
  tx: number,
  ty: number,
  direction: 'left' | 'right',
): [string, number, number] {
  const sign = direction === 'left' ? -1 : 1;
  const bow = sign * SAME_SIDE_BOW;
  const cpX = (direction === 'left' ? Math.min(sx, tx) : Math.max(sx, tx)) + bow;
  // cubic bezier: both control points bow to the same side
  const path = `M${sx},${sy} C${cpX},${sy} ${cpX},${ty} ${tx},${ty}`;
  // label sits halfway along the curve at the bow apex
  const labelX = cpX;
  const labelY = (sy + ty) / 2;
  return [path, labelX, labelY];
}

function CurlyEdge(props: EdgeProps) {
  const {
    id,
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
    style,
    label,
    labelStyle,
    labelBgStyle,
    selected,
  } = props;

  let edgePath: string;
  let labelX: number;
  let labelY: number;

  if (sourcePosition === Position.Left && targetPosition === Position.Left) {
    [edgePath, labelX, labelY] = buildSameSidePath(sourceX, sourceY, targetX, targetY, 'left');
  } else if (sourcePosition === Position.Right && targetPosition === Position.Right) {
    [edgePath, labelX, labelY] = buildSameSidePath(sourceX, sourceY, targetX, targetY, 'right');
  } else {
    [edgePath, labelX, labelY] = getBezierPath({
      sourceX,
      sourceY,
      sourcePosition,
      targetX,
      targetY,
      targetPosition,
      curvature: 0.25,
    });
  }

  return (
    <>
      <BaseEdge id={id} path={edgePath} style={style} interactionWidth={12} />
      {label && (
        <>
          <rect
            x={labelX - 30}
            y={labelY - 8}
            width={60}
            height={16}
            rx={3}
            fill={(labelBgStyle?.fill as string) ?? '#f8fafc'}
            fillOpacity={(labelBgStyle?.fillOpacity as number) ?? 0.9}
            pointerEvents="none"
          />
          <text
            x={labelX}
            y={labelY + 4}
            textAnchor="middle"
            fontSize={10}
            fontFamily="monospace"
            fontWeight={selected ? 600 : 400}
            fill={(labelStyle?.fill as string) ?? '#475569'}
            pointerEvents="none"
          >
            {label as string}
          </text>
        </>
      )}
    </>
  );
}

const edgeTypes = { default: CurlyEdge };

// ── Layout ────────────────────────────────────────────────────────────────────
// Device nodes are laid out from the live placement data (no hardcoded racks):
// devices are distributed across a roughly-square set of columns, each column
// stacked top-to-bottom by the actual rendered height of its devices. ReactFlow's
// `fitView` then frames whatever was produced.

const COL_SPACING = 320; // horizontal gap between device columns (leaves room for edges)
const DEVICE_GAP = 28; // vertical gap between stacked devices
const LAYOUT_TOP = 20;
const LAYOUT_LEFT = 20;

type DeviceType = 'server' | 'switch' | 'patch' | 'pdu';

/** Infers a device category from its ports and name for the node badge/colour. */
function inferDeviceType(name: string, ports: Port[]): DeviceType {
  if (ports.some((p) => p.type === 'power-outlet')) return 'pdu';
  const n = name.toLowerCase();
  if (/(^|[^a-z])(sw|switch|tor|leaf|spine)/.test(n)) return 'switch';
  if (n.includes('patch') || n.includes('pp')) return 'patch';
  return 'server';
}

function buildNodes(
  devices: DeviceInfo[],
  devicePorts: Record<string, Port[]>,
  usedPortIds: Set<string>,
  pendingSourcePortId: string | null,
  activeSourceType: PortType | null,
  onBodyClick: () => void,
  onHover: (id: string | null) => void,
  onEditPorts: (deviceId: string) => void,
  onPortClick: (deviceId: string, portId: string, isFree: boolean) => void,
): Node[] {
  // Fall back to the keys of devicePorts when no device list is supplied so the
  // graph still renders (labels degrade to the device id).
  const list: DeviceInfo[] =
    devices.length > 0 ? devices : Object.keys(devicePorts).map((id) => ({ id, name: id }));
  if (list.length === 0) return [];

  const cols = Math.max(1, Math.ceil(Math.sqrt(list.length)));
  const perCol = Math.ceil(list.length / cols);
  const yByCol: number[] = new Array(cols).fill(LAYOUT_TOP);

  return list.map((d, i) => {
    const col = Math.floor(i / perCol);
    const ports = devicePorts[d.id] ?? [];
    const y = yByCol[col];
    yByCol[col] = y + computeDeviceHeight(ports) + DEVICE_GAP;
    return {
      id: d.id,
      type: 'device',
      position: { x: LAYOUT_LEFT + col * COL_SPACING, y },
      draggable: true,
      data: {
        label: d.name,
        deviceType: inferDeviceType(d.name, ports),
        ports,
        nodeId: d.id,
        usedPortIds,
        pendingSourcePortId,
        activeSourceType,
        onBodyClick,
        onHover,
        onEditPorts,
        onPortClick,
      },
    };
  });
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
      <div
        style={{
          borderTop: '1px solid #e2e8f0',
          marginTop: 3,
          paddingTop: 5,
          display: 'flex',
          flexDirection: 'column',
          gap: 3,
        }}
      >
        <PortTypeLegendRow color={PORT_TYPE_STYLE['network-interface'].used} label="Network" />
        <PortTypeLegendRow color={PORT_TYPE_STYLE['console-port'].used} label="Console" />
        <PortTypeLegendRow color={PORT_TYPE_STYLE['power-port'].used} label="Power port" />
        <PortTypeLegendRow color={PORT_TYPE_STYLE['power-outlet'].used} label="Power outlet" />
      </div>
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

function PortTypeLegendRow({ color, label }: { color: string; label: string }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      <span
        style={{
          width: 10,
          height: 10,
          borderRadius: 2,
          background: color,
          flexShrink: 0,
          display: 'inline-block',
        }}
      />
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
  currentStatus: CableStatus | undefined;
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
  style.textContent = `@keyframes pulseDot { 0%,100% { box-shadow: 0 0 0 3px #c7d2fe; } 50% { box-shadow: 0 0 0 5px #e0e7ff; } }`;
  document.head.appendChild(style);
}

export function PatchMappingFlow({
  cables,
  devicePorts,
  devices,
  selectedCableId,
  filterStatus,
  filterType,
  onCableClick,
  onConnectionMade,
  onEditPorts,
  onCableStatusChange,
}: PatchMappingFlowProps) {
  const [hoveredDeviceId, setHoveredDeviceId] = useState<string | null>(null);
  const [pendingSource, setPendingSource] = useState<{ deviceId: string; portId: string } | null>(
    null,
  );
  const [draggingSourceType, setDraggingSourceType] = useState<PortType | null>(null);
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
        const cable = cables.find((c) => c.aSide.portId === portId || c.bSide.portId === portId);
        if (cable) onCableClick(cable.id);
        return;
      }
      if (!pendingSource) {
        setPendingSource({ deviceId, portId });
        return;
      }
      if (pendingSource.deviceId === deviceId) {
        setPendingSource(null);
        return;
      }
      const sourcePort = devicePorts[pendingSource.deviceId]?.find(
        (p) => p.id === pendingSource.portId,
      );
      const targetPort = devicePorts[deviceId]?.find((p) => p.id === portId);
      if (sourcePort && targetPort && !portsAreCompatible(sourcePort.type, targetPort.type)) {
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
    [pendingSource, cables, devicePorts, onCableClick, onConnectionMade],
  );

  const onBodyClick = useCallback(() => setPendingSource(null), []);

  const onConnectStart = useCallback(
    (_: unknown, { handleId }: { handleId: string | null }) => {
      if (!handleId) return;
      const port = Object.values(devicePorts)
        .flat()
        .find((p) => p.id === handleId);
      setDraggingSourceType(port?.type ?? null);
    },
    [devicePorts],
  );

  const onConnectEnd = useCallback(() => setDraggingSourceType(null), []);

  const pendingSourceType = useMemo(() => {
    if (!pendingSource) return null;
    return (
      devicePorts[pendingSource.deviceId]?.find((p) => p.id === pendingSource.portId)?.type ?? null
    );
  }, [pendingSource, devicePorts]);

  const activeSourceType = draggingSourceType ?? pendingSourceType;

  const isValidConnection = useCallback(
    (connection: Connection) => {
      if (!connection.sourceHandle || !connection.targetHandle) return true;
      if (usedPortIds.has(connection.sourceHandle) || usedPortIds.has(connection.targetHandle))
        return false;
      const allPorts = Object.values(devicePorts).flat();
      const sourcePort = allPorts.find((p) => p.id === connection.sourceHandle);
      const targetPort = allPorts.find((p) => p.id === connection.targetHandle);
      if (!sourcePort || !targetPort) return true;
      return portsAreCompatible(sourcePort.type, targetPort.type);
    },
    [devicePorts, usedPortIds],
  );

  const nodes = useMemo(
    () =>
      buildNodes(
        devices,
        devicePorts,
        usedPortIds,
        pendingSource?.portId ?? null,
        activeSourceType,
        onBodyClick,
        setHoveredDeviceId,
        onEditPorts,
        onPortClick,
      ),
    [
      devices,
      devicePorts,
      usedPortIds,
      pendingSource,
      activeSourceType,
      onBodyClick,
      onEditPorts,
      onPortClick,
    ],
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
      setPendingSource(null);
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

  // Look up pending port/device info for the status panel
  const pendingPortName = pendingSource
    ? (devicePorts[pendingSource.deviceId]?.find((p) => p.id === pendingSource.portId)?.name ??
      pendingSource.portId)
    : null;
  const pendingDeviceLabel = pendingSource
    ? (devices.find((d) => d.id === pendingSource.deviceId)?.name ?? pendingSource.deviceId)
    : null;

  return (
    <>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onEdgesChange={onEdgesChange}
        onEdgeClick={onEdgeClick}
        onEdgeContextMenu={onEdgeContextMenu}
        onConnect={onConnect}
        onConnectStart={onConnectStart}
        onConnectEnd={onConnectEnd}
        isValidConnection={isValidConnection}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
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
              <span>
                Completing cable from{' '}
                <strong style={{ fontFamily: 'monospace' }}>{pendingPortName}</strong>
                {' on '}
                <strong style={{ fontFamily: 'monospace' }}>{pendingDeviceLabel}</strong>
                {' — click a free port on another device'}
              </span>
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
