// Display mapping for PluginInstallation status phases.
// Phases come from the backend CRD (plugin-controller/pkg/api/v1/types.go):
// Pending, Deploying, Running, Degraded, Failed, Terminating.

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
