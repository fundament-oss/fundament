// Display mapping for PluginInstallation status phases.
// Phases come from the backend CRD (plugin-controller/pkg/api/v1/types.go):
// Pending, Deploying, Running, Degraded, Failed, Terminating.

import type { PluginInstallationItem } from '../plugin-resources/types';

export interface InstallStatusDisplay {
  label: string;
  /** `color` for the `nldd-badge` that shows this status. */
  badgeColor: string;
  inProgress: boolean;
}

// In-progress phases get `mintgroen`, matching a provisioning cluster: work is
// underway, nothing is wrong. `warning`/`critical` mark states needing attention.
const STATUS_DISPLAY: Record<string, InstallStatusDisplay> = {
  Pending: { label: 'Installing…', badgeColor: 'mintgroen', inProgress: true },
  Deploying: { label: 'Installing…', badgeColor: 'mintgroen', inProgress: true },
  Running: { label: 'Installed', badgeColor: 'success', inProgress: false },
  Degraded: { label: 'Degraded', badgeColor: 'warning', inProgress: false },
  Failed: { label: 'Failed', badgeColor: 'critical', inProgress: false },
  Terminating: { label: 'Removing…', badgeColor: 'oranje', inProgress: true },
};

const UNKNOWN_DISPLAY: InstallStatusDisplay = {
  label: 'Installing…',
  badgeColor: 'mintgroen',
  inProgress: true,
};

/** An installation the controller has not picked up yet has no phase; it is as
 *  much on its way as a Pending one. */
export function installPhase(phase: string | undefined | null): string {
  return phase || 'Pending';
}

export function getInstallStatusDisplay(phase: string): InstallStatusDisplay {
  return STATUS_DISPLAY[phase] ?? UNKNOWN_DISPLAY;
}

export function isInstallInProgress(phase: string): boolean {
  return getInstallStatusDisplay(phase).inProgress;
}

export function isInstallRunning(phase: string): boolean {
  return phase === 'Running';
}

/** True while the installation is being torn down, as opposed to created. Both
 *  are "in progress", but they belong to opposite buttons. */
export function isInstallTerminating(phase: string): boolean {
  return phase === 'Terminating';
}

export function isInstallFailed(phase: string): boolean {
  return phase === 'Failed';
}

/** True when the controller rejected the installation's config against its
 *  definition's schema (the ConfigValid=False condition). Retry replays the
 *  failed CR's config verbatim, so a config-caused failure can never be
 *  retried out of — the UI must offer uninstall + reinstall instead. */
export function hasInvalidConfig(item: PluginInstallationItem): boolean {
  // status itself can be absent on a CR the controller has not picked up yet.
  return (item.status?.conditions ?? []).some(
    (condition) => condition.type === 'ConfigValid' && condition.status === 'False',
  );
}
