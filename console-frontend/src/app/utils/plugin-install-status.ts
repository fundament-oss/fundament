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

/** An installation's health as its CR's status reports it. */
export function healthOf(item: PluginInstallationItem): PluginInstallationHealth {
  return {
    phase: installPhase(item.status?.phase),
    ready: item.status?.ready ?? false,
    message: item.status?.message ?? '',
  };
}

// Running but not ready: up, yet not serving as it should. The controller sets
// ready from the phase, so this is rare, but it must not read as Installed.
const NOT_READY_DISPLAY: InstallStatusDisplay = {
  label: 'Not ready',
  badgeColor: 'warning',
  inProgress: false,
};

/** What to show for an installation that is not running as it should; null for
 *  one that is, or that has no status to go by. */
export function installIssue(
  health: PluginInstallationHealth | undefined,
): PluginInstallIssue | null {
  if (!health) return null;
  if (isInstallRunning(health.phase)) {
    return health.ready ? null : { ...NOT_READY_DISPLAY, message: health.message };
  }
  return { ...getInstallStatusDisplay(health.phase), message: health.message };
}

/** How long to wait before a screen reads these installations again: quickly
 *  while one is on its way up or down, slower while one is only Degraded or
 *  Failed (the controller retries a failed one every couple of minutes, and a
 *  degraded one changes as seldom), and null when none can change on its own. */
export function installPollDelay(
  installs: readonly { phase: string; configInvalid: boolean }[],
): number | null {
  if (installs.some((install) => isInstallInProgress(install.phase))) return 5000;
  if (installs.some((install) => shouldPollInstall(install.phase, install.configInvalid))) {
    return 20000;
  }
  return null;
}

export interface InstallTransitionNotice {
  kind: 'success' | 'error';
  text: string;
}

/** The notification for an installation going from one phase to another, or
 *  null for a change that needs none. An installation that went away is not a
 *  phase change; callers say that one themselves. */
export function installTransitionNotice(
  previous: string,
  next: string,
  plugin: string,
  cluster: string,
): InstallTransitionNotice | null {
  if (previous === next) return null;
  if (previous === 'Degraded' && isInstallRunning(next)) {
    return { kind: 'success', text: `Plugin ${plugin} is running again on cluster ${cluster}` };
  }
  if (!isInstallRunning(previous) && isInstallRunning(next)) {
    return { kind: 'success', text: `Plugin ${plugin} installed on cluster ${cluster}` };
  }
  if (!isInstallFailed(previous) && isInstallFailed(next)) {
    // From Running or Degraded it was installed, and stopped working since.
    const wasUp = isInstallRunning(previous) || previous === 'Degraded';
    return {
      kind: 'error',
      text: wasUp
        ? `Plugin ${plugin} failed on cluster ${cluster}`
        : `Failed to install plugin ${plugin} on cluster ${cluster}`,
    };
  }
  return null;
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
