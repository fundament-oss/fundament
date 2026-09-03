import {
  Component,
  computed,
  inject,
  signal,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { ConfigService } from '../config.service';
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
  private configService = inject(ConfigService);

  private titleService = inject(TitleService);

  private toastService = inject(ToastService);

  private route = inject(ActivatedRoute);

  private service = inject(MarketplaceService);

  plugin = signal<MarketplacePluginDetails | null>(null);

  isLoading = signal(true);

  errorMessage = signal<string | null>(null);

  // The console page for this plugin, or '' when no console is configured.
  // Installing needs an organization and a cluster, which only the
  // authenticated console has, so the storefront's "Install plugin" button
  // hands the visitor over to it. Both apps list the same appstore.plugins
  // rows, so the id in the URL is the same key on the other side.
  //
  // Composed through URL rather than string concatenation so a query or hash on
  // the configured value survives: the demo points this at a console that
  // presents by default and has to say `?present=0` to land on the plugin page.
  // An unparseable value falls back to the no-console behaviour instead of
  // throwing, which during a server render would take the whole page down.
  consoleInstallUrl = computed(() => {
    const plugin = this.plugin();
    const consoleUrl = this.configService.getConfig().consoleUrl;
    if (!plugin || !consoleUrl) return '';
    try {
      const url = new URL(consoleUrl);
      url.pathname = `${url.pathname.replace(/\/+$/, '')}/plugins/${plugin.id}`;
      return url.toString();
    } catch {
      return '';
    }
  });

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
    // Only reachable when no console is configured: this public storefront has
    // no organization/cluster context of its own, so there is nowhere to
    // install to.
    this.toastService.info(
      `Sign in to the console to install ${plugin.displayName} onto a cluster`,
    );
  }
}
