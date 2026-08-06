import {
  Component,
  computed,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
  output,
} from '@angular/core';
import SheetSyncDirective from '../sheet-sync.directive';
import DropdownSyncDirective from '../dropdown-sync.directive';
import AutofocusDirective from '../autofocus.directive';
import {
  getInstallStatusDisplay,
  isInstallInProgress,
  isInstallFailed,
  isInstallRunning,
  isInstallTerminating,
} from '../utils/plugin-install-status';

interface Cluster {
  id: string;
  name: string;
  // null when the plugin is not installed on this cluster; otherwise the
  // PluginInstallation status phase (Pending, Deploying, Running, …).
  phase: string | null;
  // The version pinned on this cluster; empty when not installed. A plugin is
  // installed per cluster, so two clusters can run different versions.
  version: string;
  running: boolean;
}

// A published definition the user can pin on install: the version they see and
// the content hash that version resolves to.
export interface PluginVersionOption {
  version: string;
  hash: string;
}

// Emitted on install: the chosen clusters plus the pinned version/hash pair.
export interface InstallSelection {
  clusterIds: string[];
  version: string;
  hash: string;
}

// Emitted on retry: a single cluster plus the currently pinned version/hash.
export interface RetrySelection {
  clusterId: string;
  version: string;
  hash: string;
}

/** The version shows from the moment a cluster has one until the removal is
 *  through: while it is being torn down the plugin is still installed at that
 *  version. Only a new install has nothing to report yet. */
function showsInstalledVersion(cluster: Cluster): boolean {
  if (!cluster.version || cluster.phase === null) return false;
  return !isInstallInProgress(cluster.phase) || isInstallTerminating(cluster.phase);
}

@Component({
  selector: 'app-install-plugin-modal',
  imports: [SheetSyncDirective, DropdownSyncDirective, AutofocusDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './install-plugin-modal.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class InstallPluginModalComponent {
  /** Human-readable plugin name (e.g. "OpenFSC"), shown to the user. Never the
   * install identifier — the caller keeps that to itself. */
  pluginDisplayName = input('');

  clusters = input<Cluster[]>([]);

  // Published versions to choose from, latest first. Empty means nothing is
  // published yet — the plugin cannot be installed.
  versions = input<PluginVersionOption[]>([]);

  // True when fetching the versions failed (as opposed to succeeding with an
  // empty list). Lets the modal distinguish a transient error from "nothing
  // published yet" so it doesn't send the user chasing a publishing problem.
  versionsError = input(false);

  show = input(false);

  closeModal = output<void>();

  // Emits the chosen clusters plus the pinned version/hash to install.
  install = output<InstallSelection>();

  // Emits the cluster ID to uninstall the plugin from.
  uninstall = output<string>();

  // Emits a cluster to retry a failed installation on, with the current pin.
  retry = output<RetrySelection>();

  /** Everything but the newest published version; the newest sits on its own at
   *  the top of the menu. */
  earlierVersions = computed(() => this.versions().slice(1));

  /** The published versions as a sentence fragment, newest first. */
  versionList = computed(() =>
    this.versions()
      .map((v) => v.version)
      .join(', '),
  );

  statusFor = getInstallStatusDisplay;

  isInProgress = isInstallInProgress;

  isFailed = isInstallFailed;

  isRunning = isInstallRunning;

  isRemoving = isInstallTerminating;

  showsVersion = showsInstalledVersion;

  onClose(): void {
    this.closeModal.emit();
  }

  /** One row, one install, at the version picked from that row's own menu. A
   *  plugin is pinned per cluster, so the version belongs to the row and not to
   *  the sheet. */
  onInstallOne(clusterId: string, option: PluginVersionOption): void {
    this.install.emit({ clusterIds: [clusterId], version: option.version, hash: option.hash });
  }

  onUninstall(clusterId: string): void {
    this.uninstall.emit(clusterId);
  }

  /** Retries at the version already pinned on that cluster, falling back to the
   *  latest published one when the failed install never recorded a version. */
  onRetry(clusterId: string, pinned: string): void {
    const option = this.versions().find((v) => v.version === pinned) ?? this.versions()[0];
    if (!option) return;
    this.retry.emit({ clusterId, version: option.version, hash: option.hash });
  }
}
