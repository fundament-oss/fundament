import {
  Component,
  inject,
  computed,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { RouterLink } from '@angular/router';
import { LoadingIndicatorComponent } from '../icons';
import { TitleService } from '../title.service';
import { OrganizationDataService } from '../organization-data.service';
import { formatDate as formatDateUtil } from '../utils/date-format';

@Component({
  selector: 'app-projects',
  imports: [RouterLink, LoadingIndicatorComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './projects.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ProjectsComponent implements OnInit {
  private titleService = inject(TitleService);

  private organizationDataService = inject(OrganizationDataService);

  isLoading = this.organizationDataService.loading;

  clusters = computed(() => {
    const orgs = this.organizationDataService.organizations();
    return orgs.flatMap((org) => org.clusters);
  });

  constructor() {
    this.titleService.setTitle('Projects');
  }

  ngOnInit() {
    // Projects are not pre-loaded on app init; load them now (deduplicates if already in flight).
    this.organizationDataService.loadProjectsAndNamespaces().catch(() => {});
  }

  readonly formatDate = formatDateUtil;
}
