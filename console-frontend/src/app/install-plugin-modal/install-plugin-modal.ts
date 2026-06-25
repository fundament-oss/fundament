import {
  Component,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  inject,
  input,
  output,
  signal,
  effect,
  viewChild,
  ElementRef,
  OnDestroy,
} from '@angular/core';
import DialogSyncDirective from '../dialog-sync.directive';
import { LoadingIndicatorComponent } from '../icons';
import focusFirstModalInput from '../modal-focus';
import { ToastService } from '../toast.service';
import PluginInstallationService from '../plugin-installation/plugin-installation.service';

interface Cluster {
  id: string;
  name: string;
  installed: boolean;
  running: boolean;
}

type InstallState = 'idle' | 'installing' | 'installed' | 'failed';

const POLL_INTERVAL_MS = 3000;
const POLL_TIMEOUT_MS = 120000;

@Component({
  selector: 'app-install-plugin-modal',
  imports: [DialogSyncDirective, LoadingIndicatorComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './install-plugin-modal.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class InstallPluginModalComponent implements OnDestroy {
  private installationService = inject(PluginInstallationService);

  private toastService = inject(ToastService);

  pluginName = input('');

  image = input('');

  clusters = input<Cluster[]>([]);

  show = input(false);

  closeModal = output<void>();

  /** Emitted right after the install request is accepted, so parents can update their cache. */
  installed = output<string>();

  /** Per-cluster live install state, keyed by cluster id. Reset whenever the modal closes. */
  private installStates = signal<Record<string, InstallState>>({});

  private pollTimers = new Map<string, ReturnType<typeof setTimeout>>();

  dialogRef = viewChild<ElementRef<HTMLElement>>('dialog');

  constructor() {
    // Stop polling and clear transient state whenever the modal closes, so the
    // next open starts fresh from the clusters' own installed flags.
    effect(() => {
      if (!this.show()) {
        this.stopAllPolling();
        this.installStates.set({});
      }
    });
  }

  ngOnDestroy(): void {
    this.stopAllPolling();
  }

  stateFor(cluster: Cluster): InstallState {
    return this.installStates()[cluster.id] ?? (cluster.installed ? 'installed' : 'idle');
  }

  private setState(clusterId: string, state: InstallState): void {
    this.installStates.update((states) => ({ ...states, [clusterId]: state }));
  }

  onOpen(): void {
    const el = this.dialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }

  onClose(): void {
    this.closeModal.emit();
  }

  async onInstall(clusterId: string): Promise<void> {
    const cluster = this.clusters().find((c) => c.id === clusterId);
    if (!cluster) return;

    this.setState(clusterId, 'installing');

    try {
      await this.installationService.installPlugin(clusterId, this.pluginName(), this.image());
    } catch {
      this.setState(clusterId, 'failed');
      this.toastService.error(`Failed to install ${this.pluginName()} on ${cluster.name}`);
      return;
    }

    this.installed.emit(clusterId);
    this.pollInstallation(clusterId, cluster.name);
  }

  private pollInstallation(clusterId: string, clusterName: string): void {
    const deadline = Date.now() + POLL_TIMEOUT_MS;

    const tick = async () => {
      let ready = false;
      let failed = false;
      try {
        const installations = await this.installationService.listInstallations(clusterId);
        const item = installations.find((i) => i.metadata.name === this.pluginName());
        if (item) {
          ready = item.status?.ready === true;
          failed = item.status?.phase?.toLowerCase() === 'failed';
        }
      } catch {
        // Ignore transient polling errors and try again until the deadline.
      }

      if (ready) {
        this.setState(clusterId, 'installed');
        this.toastService.success(`${this.pluginName()} installed on ${clusterName}`);
        this.pollTimers.delete(clusterId);
        return;
      }
      if (failed) {
        this.setState(clusterId, 'failed');
        this.toastService.error(`Failed to install ${this.pluginName()} on ${clusterName}`);
        this.pollTimers.delete(clusterId);
        return;
      }
      if (Date.now() >= deadline) {
        // Still provisioning after the timeout — leave the row as "installing".
        this.pollTimers.delete(clusterId);
        return;
      }
      this.pollTimers.set(clusterId, setTimeout(tick, POLL_INTERVAL_MS));
    };

    this.pollTimers.set(clusterId, setTimeout(tick, POLL_INTERVAL_MS));
  }

  private stopAllPolling(): void {
    this.pollTimers.forEach((timer) => clearTimeout(timer));
    this.pollTimers.clear();
  }
}
