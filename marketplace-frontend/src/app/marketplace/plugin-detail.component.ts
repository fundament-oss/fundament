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
import MarketplaceService, { type MarketplacePluginDetails } from './marketplace.service';
import PluginLabelsComponent from './plugin-labels.component';
import connectErrorMessage from '../../connect/error';

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

  plugin = signal<MarketplacePluginDetails | null>(null);

  isLoading = signal(true);

  errorMessage = signal<string | null>(null);

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
      this.titleService.setTitle(plugin.displayName);
    } catch (error) {
      this.errorMessage.set(connectErrorMessage(error));
    } finally {
      this.isLoading.set(false);
    }
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
