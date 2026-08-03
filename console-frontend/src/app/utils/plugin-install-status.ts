// Display mapping for PluginInstallation status phases.
// Phases come from the backend CRD (plugin-controller/pkg/api/v1/types.go):
// Pending, Deploying, Running, Degraded, Failed, Terminating.

export interface InstallStatusDisplay {
  label: string;
  /** `color` for an `nldd-tag`. */
  tagColor: string;
  inProgress: boolean;
}

// In-progress phases get `mintgroen`, matching a provisioning cluster: work is
// underway, nothing is wrong. `warning`/`critical` mark states needing attention.
const STATUS_DISPLAY: Record<string, InstallStatusDisplay> = {
  Pending: { label: 'Installing…', tagColor: 'mintgroen', inProgress: true },
  Deploying: { label: 'Installing…', tagColor: 'mintgroen', inProgress: true },
  Running: { label: 'Installed', tagColor: 'success', inProgress: false },
  Degraded: { label: 'Degraded', tagColor: 'warning', inProgress: false },
  Failed: { label: 'Failed', tagColor: 'critical', inProgress: false },
  Terminating: { label: 'Removing…', tagColor: 'oranje', inProgress: true },
};

const UNKNOWN_DISPLAY: InstallStatusDisplay = {
  label: 'Installing…',
  tagColor: 'mintgroen',
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

export function isInstallFailed(phase: string): boolean {
  return phase === 'Failed';
}
