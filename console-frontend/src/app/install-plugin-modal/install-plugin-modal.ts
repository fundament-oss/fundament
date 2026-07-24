import {
  Component,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  input,
  output,
  signal,
  viewChild,
  ElementRef,
} from '@angular/core';
import DialogSyncDirective from '../dialog-sync.directive';
import focusFirstModalInput from '../modal-focus';
import { LoadingIndicatorComponent } from '../icons';
import {
  getInstallStatusDisplay,
  isInstallInProgress,
  isInstallFailed,
} from '../utils/plugin-install-status';

interface Cluster {
  id: string;
  name: string;
  // null when the plugin is not installed on this cluster; otherwise the
  // PluginInstallation status phase (Pending, Deploying, Running, …).
  phase: string | null;
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

@Component({
  selector: 'app-install-plugin-modal',
  imports: [DialogSyncDirective, LoadingIndicatorComponent],
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

  show = input(false);

  closeModal = output<void>();

  // Emits the chosen clusters plus the pinned version/hash to install.
  install = output<InstallSelection>();

  // Emits the cluster ID to uninstall the plugin from.
  uninstall = output<string>();

  // Emits a cluster to retry a failed installation on, with the current pin.
  retry = output<RetrySelection>();

  dialogRef = viewChild<ElementRef<HTMLElement>>('dialog');

  // Cluster IDs currently selected for batch install.
  selected = signal<Set<string>>(new Set());

  // The version the user picked, or null to fall back to the latest. Reset on
  // every open so re-opening defaults to the newest published version.
  private pickedVersion = signal<string | null>(null);

  // Effective selection: the user's pick when still valid, else the latest
  // (first) published version. Empty when nothing is published.
  selectedVersion = computed(() => {
    const versions = this.versions();
    const picked = this.pickedVersion();
    if (picked && versions.some((v) => v.version === picked)) return picked;
    return versions[0]?.version ?? '';
  });

  // Clusters eligible for selection: running and not yet installed.
  eligibleClusters = computed(() =>
    this.clusters().filter((cluster) => cluster.running && cluster.phase === null),
  );

  selectedCount = computed(() => this.selected().size);

  allSelected = computed(() => {
    const eligible = this.eligibleClusters();
    return eligible.length > 0 && eligible.every((cluster) => this.selected().has(cluster.id));
  });

  statusFor = getInstallStatusDisplay;

  isInProgress = isInstallInProgress;

  isFailed = isInstallFailed;

  isSelected(clusterId: string): boolean {
    return this.selected().has(clusterId);
  }

  setSelected(clusterId: string, checked: boolean): void {
    this.selected.update((current) => {
      const next = new Set(current);
      if (checked) {
        next.add(clusterId);
      } else {
        next.delete(clusterId);
      }
      return next;
    });
  }

  setAllSelected(checked: boolean): void {
    const eligible = this.eligibleClusters();
    this.selected.set(checked ? new Set(eligible.map((cluster) => cluster.id)) : new Set());
  }

  onOpen(): void {
    this.selected.set(new Set());
    this.pickedVersion.set(null);
    const el = this.dialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }

  onVersionChange(version: string): void {
    this.pickedVersion.set(version);
  }

  onClose(): void {
    this.closeModal.emit();
  }

  onInstallSelected(): void {
    const ids = [...this.selected()];
    const option = this.versions().find((v) => v.version === this.selectedVersion());
    if (ids.length === 0 || !option) return;
    this.install.emit({ clusterIds: ids, version: option.version, hash: option.hash });
    this.selected.set(new Set());
  }

  onUninstall(clusterId: string): void {
    this.uninstall.emit(clusterId);
  }

  onRetry(clusterId: string): void {
    const option = this.versions().find((v) => v.version === this.selectedVersion());
    if (!option) return;
    this.retry.emit({ clusterId, version: option.version, hash: option.hash });
  }
}
