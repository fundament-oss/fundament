import {
  Component,
  inject,
  signal,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import PluginDevelopmentService, {
  type AuthoredPlugin,
} from '../plugin-development/plugin-development.service';
import {
  statusLabel,
  statusBadgeClass,
  tierLabel,
  tierBadgeClass,
} from '../plugin-development/status-display';
import PluginStatusTrackerComponent from '../plugin-status-tracker/plugin-status-tracker.component';

@Component({
  selector: 'app-plugin-development-detail',
  imports: [RouterLink, PluginStatusTrackerComponent],
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
    this.isLoading.set(false);
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

  statusLabel = statusLabel;

  statusBadgeClass = statusBadgeClass;

  tierLabel = tierLabel;

  tierBadgeClass = tierBadgeClass;
}
