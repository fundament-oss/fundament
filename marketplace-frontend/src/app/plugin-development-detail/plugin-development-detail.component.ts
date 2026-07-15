import {
  Component,
  inject,
  signal,
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
  type SideloadCluster,
} from '../plugin-development/plugin-development.service';
import {
  statusLabel,
  statusTagColor,
  statusBadgeClass,
} from '../plugin-development/status-display';
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

  plugin = signal<AuthoredPlugin | null>(null);

  isLoading = signal(true);

  errorMessage = signal<string | null>(null);

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
    const name = this.route.snapshot.paramMap.get('name');
    if (!name) {
      this.errorMessage.set('Plugin name is missing');
      this.isLoading.set(false);
      return;
    }

    const plugin = await this.service.getPlugin(name);
    if (!plugin) {
      this.errorMessage.set('Plugin not found');
      this.isLoading.set(false);
      return;
    }

    this.plugin.set(plugin);
    this.titleService.setTitle(`${plugin.displayName} — My plugins`);
    this.selectedVersion.set(plugin.version);
    this.isLoading.set(false);

    const clusters = await this.service.listClusters();
    this.clusters.set(clusters);
    // Default to the first cluster.
    if (clusters[0]) {
      this.selectedClusterId.set(clusters[0].id);
    }
  }

  submitForReview() {
    const plugin = this.plugin();
    if (!plugin) return;
    this.toastService.success(`${plugin.displayName} v${plugin.version} submitted for review`);
  }

  withdraw() {
    const plugin = this.plugin();
    if (!plugin) return;
    this.toastService.info(`Withdrew ${plugin.displayName} from review`);
  }

  unpublish() {
    const plugin = this.plugin();
    if (!plugin) return;
    this.toastService.info(`${plugin.displayName} unpublished from the catalog`);
  }

  viewInCatalog() {
    this.toastService.info('This is a mockup — the catalog listing is not wired up yet');
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

    // Point the image at the selected build by swapping its tag.
    const image = `${plugin.image.replace(/:[^:]*$/, '')}:${version}`;

    await this.service.sideload({
      image,
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
