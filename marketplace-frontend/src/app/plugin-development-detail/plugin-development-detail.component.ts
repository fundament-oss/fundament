import {
  Component,
  inject,
  signal,
  computed,
  effect,
  viewChild,
  ElementRef,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { PluginIconComponent } from '../icons';
import PluginDevelopmentService, {
  type AuthoredPlugin,
} from '../plugin-development/plugin-development.service';
import SideloadMockService, {
  type SideloadCluster,
} from '../plugin-development/sideload-mock.service';
import { statusLabel, statusTagColor, statusBadgeClass } from '../status/submission-status';
import connectErrorMessage from '../../connect/error';
import PluginStatusTrackerComponent from '../plugin-status-tracker/plugin-status-tracker.component';

@Component({
  selector: 'app-plugin-development-detail',
  imports: [RouterLink, PluginStatusTrackerComponent, PluginIconComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './plugin-development-detail.component.html',
})
export default class PluginDevelopmentDetailComponent implements OnInit {
  private titleService = inject(TitleService);

  private toastService = inject(ToastService);

  private route = inject(ActivatedRoute);

  private service = inject(PluginDevelopmentService);

  // Sideloading belongs to organization-api, not to the marketplace APIs, so it
  // stays mocked (see sideload-mock.service.ts).
  private sideloadService = inject(SideloadMockService);

  plugin = signal<AuthoredPlugin | null>(null);

  isLoading = signal(true);

  errorMessage = signal<string | null>(null);

  // Guards the review actions while an RPC is in flight.
  isSubmitting = signal(false);

  // The newest pushed build: the one every review action acts on, since the
  // reviewed unit is a version.
  latestVersion = computed(() => this.plugin()?.versions[0] ?? null);

  // Clusters the author can sideload onto, and the currently selected target.
  clusters = signal<SideloadCluster[]>([]);

  selectedClusterId = signal<string>('');

  // Which pushed build to sideload; defaults to the latest version.
  selectedVersion = signal<string>('');

  // Controls visibility of the right-hand sideload sheet.
  showSideloadSheet = signal(false);

  private readonly sideloadSheetEl = viewChild<ElementRef>('sideloadSheet');

  constructor() {
    effect(() => {
      const el = this.sideloadSheetEl()?.nativeElement as {
        show?: () => void;
        hide?: () => void;
      };
      if (this.showSideloadSheet()) el?.show?.();
      else el?.hide?.();
    });
  }

  async ngOnInit() {
    const id = this.route.snapshot.paramMap.get('id');
    if (!id) {
      this.errorMessage.set('Plugin id is missing');
      this.isLoading.set(false);
      return;
    }

    try {
      const plugin = await this.service.getPlugin(id);
      if (!plugin) {
        this.errorMessage.set('Plugin not found');
        return;
      }
      this.plugin.set(plugin);
      this.titleService.setTitle(`${plugin.displayName} — My plugins`);
      this.selectedVersion.set(plugin.version);
    } catch (error) {
      this.errorMessage.set(connectErrorMessage(error));
      return;
    } finally {
      this.isLoading.set(false);
    }

    const clusters = await this.sideloadService.listClusters();
    this.clusters.set(clusters);
    // Default to the first cluster.
    if (clusters[0]) {
      this.selectedClusterId.set(clusters[0].id);
    }
  }

  async submitForReview() {
    const plugin = this.plugin();
    const version = this.latestVersion();
    if (!plugin || !version) return;

    this.isSubmitting.set(true);
    try {
      await this.service.submitVersion(version.id);
      await this.reload(plugin.id);
      this.toastService.success(`${plugin.displayName} ${version.version} submitted for review`);
    } catch (error) {
      this.toastService.error(connectErrorMessage(error));
    } finally {
      this.isSubmitting.set(false);
    }
  }

  async withdraw() {
    const plugin = this.plugin();
    const version = this.latestVersion();
    if (!plugin || !version) return;

    this.isSubmitting.set(true);
    try {
      await this.service.withdrawVersion(version.id);
      await this.reload(plugin.id);
      this.toastService.info(`Withdrew ${plugin.displayName} ${version.version} from review`);
    } catch (error) {
      this.toastService.error(connectErrorMessage(error));
    } finally {
      this.isSubmitting.set(false);
    }
  }

  // The decision lands on the version, so the whole listing is re-read rather
  // than patched: its status is derived from the versions it holds.
  private async reload(id: string) {
    const plugin = await this.service.getPlugin(id);
    if (plugin) this.plugin.set(plugin);
  }

  openSideload() {
    this.showSideloadSheet.set(true);
  }

  closeSideload() {
    this.showSideloadSheet.set(false);
  }

  onClusterChange(event: Event) {
    this.selectedClusterId.set((event.target as HTMLSelectElement).value);
  }

  onVersionChange(event: Event) {
    this.selectedVersion.set((event.target as HTMLSelectElement).value);
  }

  async submitSideload() {
    const plugin = this.plugin();
    const clusterId = this.selectedClusterId();
    const version = this.selectedVersion();
    if (!plugin || !clusterId || !version) return;

    // The container image is a property of the build, read out of its pinned
    // manifest, so the selected version carries the exact image to sideload.
    const build = plugin.versions.find((candidate) => candidate.version === version);
    if (!build) return;

    await this.sideloadService.sideload({
      image: build.image,
      version,
      displayName: plugin.displayName,
      description: plugin.descriptionShort,
      clusterId,
    });

    const cluster = this.clusters().find((c) => c.id === clusterId);
    this.toastService.success(
      `Sideloading ${plugin.displayName} v${version} onto ${cluster?.name ?? 'the selected cluster'}`,
    );
    this.closeSideload();
  }

  statusLabel = statusLabel;

  statusTagColor = statusTagColor;

  statusBadgeClass = statusBadgeClass;
}
