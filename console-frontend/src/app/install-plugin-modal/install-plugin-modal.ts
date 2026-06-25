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

@Component({
  selector: 'app-install-plugin-modal',
  imports: [DialogSyncDirective, LoadingIndicatorComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './install-plugin-modal.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class InstallPluginModalComponent {
  pluginName = input('');

  clusters = input<Cluster[]>([]);

  show = input(false);

  closeModal = output<void>();

  // Emits the cluster IDs to install the plugin on (batch).
  install = output<string[]>();

  // Emits the cluster ID to uninstall the plugin from.
  uninstall = output<string>();

  // Emits the cluster ID to retry a failed installation on.
  retry = output<string>();

  dialogRef = viewChild<ElementRef<HTMLElement>>('dialog');

  // Cluster IDs currently selected for batch install.
  selected = signal<Set<string>>(new Set());

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
    const el = this.dialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }

  onClose(): void {
    this.closeModal.emit();
  }

  onInstallSelected(): void {
    const ids = [...this.selected()];
    if (ids.length === 0) return;
    this.install.emit(ids);
    this.selected.set(new Set());
  }

  onUninstall(clusterId: string): void {
    this.uninstall.emit(clusterId);
  }

  onRetry(clusterId: string): void {
    this.retry.emit(clusterId);
  }
}
