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
import PluginDevelopmentService, { type AuthoredPlugin } from './plugin-development.service';
import {
  type SubmissionStatus,
  statusLabel,
  statusTagColor,
  statusBadgeClass,
} from '../status/submission-status';
import connectErrorMessage from '../../connect/error';

interface StatusSummary {
  status: SubmissionStatus;
  label: string;
  count: number;
  dotColorVar: string;
}

// The four states a version passes through on its way to the catalog. Rejected
// and withdrawn versions still show their status in the table; they do not get
// a counter, because neither is a stage a developer works through.
const SUMMARY_STATUSES: { status: SubmissionStatus; dotColorVar: string }[] = [
  { status: 'draft', dotColorVar: 'var(--primitives-color-neutral-500)' },
  { status: 'pending', dotColorVar: 'var(--primitives-color-accent-650)' },
  { status: 'changes_requested', dotColorVar: 'var(--primitives-color-warning-600)' },
  { status: 'approved', dotColorVar: 'var(--primitives-color-success-600)' },
];

@Component({
  selector: 'app-plugin-development',
  imports: [RouterLink, PluginIconComponent],
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

  errorMessage = signal<string | null>(null);

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
    try {
      this.plugins.set(await this.service.listPlugins());
    } catch (error) {
      this.errorMessage.set(connectErrorMessage(error));
    } finally {
      this.isLoading.set(false);
    }
  }

  statusLabel = statusLabel;

  statusTagColor = statusTagColor;

  statusBadgeClass = statusBadgeClass;

  goToManage(id: string) {
    this.router.navigate(['/manage', id]);
  }
}
