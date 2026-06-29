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
import PluginInstallationService, {
  pluginResourceName,
} from '../plugin-installation/plugin-installation.service';

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

  /** Emitted once an install is confirmed ready, so parents only cache successful installs. */
  installed = output<string>();

  /** Per-cluster live install state, keyed by cluster id. Reset whenever the modal closes. */
  private installStates = signal<Record<string, InstallState>>({});

  private pollTimers = new Map<string, ReturnType<typeof setTimeout>>();

  // Bumped whenever polling is stopped (e.g. the modal closes). A poll tick that
  // was already in flight captures the generation it started under and bails on
  // resume if it no longer matches, so it can't resurrect state or fire a toast
  // for a closed modal.
  private pollGeneration = 0;

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

    const pluginName = this.pluginName();
    if (!pluginName || this.stateFor(cluster) === 'installing') return;

    const image = this.image();
    this.setState(clusterId, 'installing');

    try {
      await this.installationService.installPlugin(clusterId, pluginName, image);
    } catch {
      this.setState(clusterId, 'failed');
      this.toastService.error(`Failed to install ${pluginName} on ${cluster.name}`);
      return;
    }

    // Don't start a background poll for a modal the user already closed.
    if (this.show()) this.pollInstallation(clusterId, cluster.name, pluginName);
  }

  private pollInstallation(clusterId: string, clusterName: string, pluginName: string): void {
    const deadline = Date.now() + POLL_TIMEOUT_MS;
    const generation = this.pollGeneration;

    const tick = async () => {
      let ready = false;
      let failed = false;
      try {
        const item = await this.installationService.getInstallation(
          clusterId,
          pluginResourceName(pluginName),
        );
        if (item) {
          ready = item.status?.ready === true;
          failed = item.status?.phase?.toLowerCase() === 'failed';
        }
      } catch {
        // Ignore transient polling errors and try again until the deadline.
      }

      // The modal closed (or a newer poll session started) while this tick was
      // in flight — drop it without touching state, toasts, or timers.
      if (generation !== this.pollGeneration) return;

      if (ready) {
        this.setState(clusterId, 'installed');
        this.installed.emit(clusterId);
        this.toastService.success(`${pluginName} installed on ${clusterName}`);
        this.pollTimers.delete(clusterId);
        return;
      }
      if (failed) {
        this.setState(clusterId, 'failed');
        this.toastService.error(`Failed to install ${pluginName} on ${clusterName}`);
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
    this.pollGeneration++;
    this.pollTimers.forEach((timer) => clearTimeout(timer));
    this.pollTimers.clear();
  }
}
