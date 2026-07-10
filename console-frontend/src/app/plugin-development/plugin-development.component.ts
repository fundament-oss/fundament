import {
  Component,
  inject,
  signal,
  computed,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { PluginIconComponent } from '../icons';
import PluginDevelopmentService, {
  type AuthoredPlugin,
  type PluginStatus,
} from './plugin-development.service';
import { statusLabel, statusTagColor, statusBadgeClass } from './status-display';
import PluginNavTabsComponent from './plugin-nav-tabs.component';

interface StatusSummary {
  status: PluginStatus;
  label: string;
  count: number;
  dotColorVar: string;
}

const SUMMARY_STATUSES: { status: PluginStatus; dotColorVar: string }[] = [
  { status: 'published', dotColorVar: 'var(--primitives-color-success-600)' },
  { status: 'in_review', dotColorVar: 'var(--primitives-color-accent-650)' },
  { status: 'changes_requested', dotColorVar: 'var(--primitives-color-warning-600)' },
  { status: 'pushed', dotColorVar: 'var(--primitives-color-neutral-500)' },
];

@Component({
  selector: 'app-plugin-development',
  imports: [RouterLink, PluginNavTabsComponent, PluginIconComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './plugin-development.component.html',
})
export default class PluginDevelopmentComponent implements OnInit {
  private titleService = inject(TitleService);

  private service = inject(PluginDevelopmentService);

  private router = inject(Router);

  plugins = signal<AuthoredPlugin[]>([]);

  isLoading = signal(true);

  statusCounts = computed<StatusSummary[]>(() => {
    const plugins = this.plugins();
    return SUMMARY_STATUSES.map(({ status, dotColorVar }) => ({
      status,
      label: statusLabel(status),
      count: plugins.filter((plugin) => plugin.status === status).length,
      dotColorVar,
    }));
  });

  constructor() {
    this.titleService.setTitle('My plugins');
  }

  async ngOnInit() {
    this.plugins.set(await this.service.listPlugins());
    this.isLoading.set(false);
  }

  statusLabel = statusLabel;

  statusTagColor = statusTagColor;

  statusBadgeClass = statusBadgeClass;

  goToManage(name: string) {
    this.router.navigate(['/plugins/manage', name]);
  }
}
