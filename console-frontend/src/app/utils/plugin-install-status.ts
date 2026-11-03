// Display mapping for PluginInstallation status phases.
// Phases come from the backend CRD (plugin-controller/pkg/api/v1/types.go):
// Pending, Deploying, Running, Degraded, Failed, Terminating.

import type {
  PluginInstallationHealth,
  PluginInstallationItem,
  PluginInstallIssue,
} from '../plugin-resources/types';

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

/** True when the installation is up but not working (Degraded) or did not come
 *  up at all (Failed). Neither is in progress, yet both can change on their
 *  own: the plugin recovers, or the controller's retry gets it going. */
export function isInstallUnhealthy(phase: string): boolean {
  return phase === 'Degraded' || isInstallFailed(phase);
}

/** True while the installation's phase may still change without anyone acting
 *  on it, so a screen showing it should keep reading it. A failure caused by
 *  its config (see hasInvalidConfig) is not one: it stays Failed until someone
 *  uninstalls and reinstalls. */
export function shouldPollInstall(phase: string, configInvalid: boolean): boolean {
  if (configInvalid && isInstallFailed(phase)) return false;
  return isInstallInProgress(phase) || isInstallUnhealthy(phase);
}

/** What to show for an installation that is not running as it should; null for
 *  one that is, or that has no status to go by. */
export function installIssue(
  health: PluginInstallationHealth | undefined,
): PluginInstallIssue | null {
  if (!health || (isInstallRunning(health.phase) && health.ready)) return null;
  const display = getInstallStatusDisplay(health.phase);
  return {
    label: display.label,
    badgeColor: display.badgeColor,
    inProgress: display.inProgress,
    message: health.message,
  };
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
