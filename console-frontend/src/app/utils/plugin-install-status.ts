// Display mapping for PluginInstallation status phases.
// Phases come from the backend CRD (plugin-controller/pkg/api/v1/types.go):
// Pending, Deploying, Running, Degraded, Failed, Terminating.

export interface InstallStatusDisplay {
  label: string;
  badgeClass: string;
  inProgress: boolean;
}

const STATUS_DISPLAY: Record<string, InstallStatusDisplay> = {
  Pending: { label: 'Installing…', badgeClass: 'badge-blue', inProgress: true },
  Deploying: { label: 'Installing…', badgeClass: 'badge-blue', inProgress: true },
  Running: { label: 'Installed', badgeClass: 'badge-emerald', inProgress: false },
  Degraded: { label: 'Degraded', badgeClass: 'badge-yellow', inProgress: false },
  Failed: { label: 'Failed', badgeClass: 'badge-rose', inProgress: false },
  Terminating: { label: 'Removing…', badgeClass: 'badge-gray', inProgress: true },
};

const UNKNOWN_DISPLAY: InstallStatusDisplay = {
  label: 'Installing…',
  badgeClass: 'badge-blue',
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
