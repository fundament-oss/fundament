import {
  Component,
  inject,
  signal,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import PluginDevelopmentService, { type AuthoredPlugin } from './plugin-development.service';
import {
  statusLabel,
  statusBadgeClass,
  tierLabel,
  tierBadgeClass,
} from './status-display';
import PluginNavTabsComponent from './plugin-nav-tabs.component';

@Component({
  selector: 'app-plugin-development',
  imports: [RouterLink, PluginNavTabsComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './plugin-development.component.html',
})
export default class PluginDevelopmentComponent implements OnInit {
  private titleService = inject(TitleService);

  private service = inject(PluginDevelopmentService);

  plugins = signal<AuthoredPlugin[]>([]);

  isLoading = signal(true);

  constructor() {
    this.titleService.setTitle('My plugins');
  }

  async ngOnInit() {
    this.plugins.set(await this.service.listPlugins());
    this.isLoading.set(false);
  }

  statusLabel = statusLabel;

  statusBadgeClass = statusBadgeClass;

  tierLabel = tierLabel;

  tierBadgeClass = tierBadgeClass;
}
