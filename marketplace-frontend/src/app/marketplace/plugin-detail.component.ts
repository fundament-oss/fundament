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
import { PluginIconComponent } from '../icons';
import MarketplaceService, { type MarketplacePlugin } from './marketplace.service';
import PluginLabelsComponent from './plugin-labels.component';

@Component({
  selector: 'app-plugin-detail',
  imports: [RouterLink, PluginIconComponent, PluginLabelsComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './plugin-detail.component.html',
})
export default class PluginDetailComponent implements OnInit {
  private titleService = inject(TitleService);

  private toastService = inject(ToastService);

  private route = inject(ActivatedRoute);

  private service = inject(MarketplaceService);

  plugin = signal<MarketplacePlugin | null>(null);

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
    this.titleService.setTitle(plugin.displayName);
    this.isLoading.set(false);
  }

  install() {
    const plugin = this.plugin();
    if (!plugin) return;
    // This public storefront has no organization/cluster context, so installing
    // is a mock action. A real install happens from the authenticated console.
    this.toastService.info(
      `Sign in to the console to install ${plugin.displayName} onto a cluster`,
    );
  }
}
