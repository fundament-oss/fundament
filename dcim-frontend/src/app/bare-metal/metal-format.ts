import type { Tag } from '../platform/fleet-format';
import type {
  ComponentStatus,
  IssueSeverity,
  Machine,
  MachineState,
  Network,
  Switch,
} from './metal-metrics';

/** Watts, whole. The exporters publish a gauge in watts. */
export function formatWatts(watts: number): string {
  return `${Math.round(watts)} W`;
}

/** The estate's draw in kilowatts, one decimal. */
export function formatKilowatts(watts: number): string {
  return `${(watts / 1000).toFixed(1)} kW`;
}

export function formatMilliseconds(milliseconds: number): string {
  return `${Math.round(milliseconds)} ms`;
}

/** A latency from a histogram, in the unit that keeps it readable. */
export function formatSeconds(seconds: number): string {
  if (seconds < 1) return `${Math.round(seconds * 1000)} ms`;
  return `${Number(seconds.toFixed(seconds < 10 ? 2 : 1))} s`;
}

export function formatRate(perMinute: number): string {
  return `${perMinute < 10 ? perMinute.toFixed(1) : Math.round(perMinute)}/min`;
}

/** "30 d", "8 d", "12 h", "40 min": how long something still lasts. */
export function formatDuration(seconds: number): string {
  if (seconds < 3600) return `${Math.max(1, Math.round(seconds / 60))} min`;
  if (seconds < 86400) return `${Math.round(seconds / 3600)} h`;
  return `${Math.round(seconds / 86400)} d`;
}

/** The snapshot's clock, in UTC, without the seconds. */
export function formatTimestamp(at: Date): string {
  return `${at.toISOString().slice(0, 16).replace('T', ' ')} UTC`;
}

export function formatCount(value: number): string {
  return String(Math.round(value));
}

export function machineStateTag(state: MachineState): Tag {
  switch (state) {
    case 'available':
      return { text: 'Available', color: 'success' };
    case 'reserved':
      return { text: 'Reserved', color: 'warning' };
    case 'locked':
      return { text: 'Locked', color: 'neutral' };
    case 'other':
      return { text: 'Other', color: 'neutral' };
    default: {
      const unhandled: never = state;
      throw new Error(`unhandled machine state ${JSON.stringify(unhandled)}`);
    }
  }
}

export function issueSeverityTag(severity: IssueSeverity): Tag {
  switch (severity) {
    case 'critical':
      return { text: 'Critical', color: 'critical' };
    case 'major':
      return { text: 'Major', color: 'warning' };
    case 'minor':
      return { text: 'Minor', color: 'neutral' };
    default: {
      const unhandled: never = severity;
      throw new Error(`unhandled issue severity ${JSON.stringify(unhandled)}`);
    }
  }
}

/** How many issues a machine has, in the colour of its worst one. */
export function machineIssueTag(machine: Machine): Tag | null {
  if (machine.issues.length === 0) return null;
  const critical = machine.issues.some((issue) => issue.severity === 'critical');
  return { text: String(machine.issues.length), color: critical ? 'critical' : 'warning' };
}

export function componentTag(component: ComponentStatus): Tag {
  return component.up ? { text: 'Up', color: 'success' } : { text: 'Down', color: 'critical' };
}

export function syncTag(syncFailing: boolean): Tag {
  return syncFailing
    ? { text: 'Failing', color: 'critical' }
    : { text: 'In sync', color: 'success' };
}

/** A token that expires within a week is worth a warning before it does. */
export function tokenTag(seconds: number | null): Tag | null {
  if (seconds === null || seconds >= 7 * 86400) return null;
  return { text: 'Expires soon', color: 'warning' };
}

// ── Table cells ─────────────────────────────────────────────────────────────
// The values that are empty rather than zero, which is what the exporter's
// NOTALLOCATED sentinel and a missing BMC report really mean.

export function machineName(machine: Machine): string {
  return machine.name || 'Not allocated';
}

export function machineRole(machine: Machine): string {
  return machine.role === 'none' ? '' : machine.role;
}

export function powerText(machine: Machine): string {
  return machine.powerWatts === null ? '' : formatWatts(machine.powerWatts);
}

export function psuText(machine: Machine): string {
  return machine.psuTotal === 0 ? '' : `${machine.psuHealthy} / ${machine.psuTotal}`;
}

export function ipText(network: Network): string {
  return `${network.ipsUsed} / ${network.ipsUsed + network.ipsAvailable}`;
}

export function prefixText(network: Network): string {
  return `${network.prefixesUsed} / ${network.prefixesUsed + network.prefixesAvailable}`;
}

export function bgpText(item: Switch): string {
  return `${item.bgpEstablished} / ${item.bgpTotal}`;
}

export function tokenText(component: ComponentStatus): string {
  return component.tokenLifetimeSeconds === null
    ? ''
    : formatDuration(component.tokenLifetimeSeconds);
}

export function componentType(component: ComponentStatus): string {
  return component.type || component.identifier;
}
