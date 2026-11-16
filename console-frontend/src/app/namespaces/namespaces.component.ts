import {
  Component,
  inject,
  signal,
  computed,
  OnInit,
  effect,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ActivatedRoute, Router, RouterOutlet } from '@angular/router';
import { ReactiveFormsModule, FormBuilder } from '@angular/forms';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { createIdempotencyRef } from '../../connect/idempotency';
import { TitleService } from '../title.service';
import { NotificationService } from '../notification.service';
import PageNavService from '../page-nav.service';
import { OverlayService } from '../overlay.service';
import { OrganizationDataService } from '../organization-data.service';
import { ListProjectNamespacesRequestSchema, Namespace } from '../../generated/v1/namespace_pb';
import DialogSyncDirective from '../dialog-sync.directive';
import { formatDate as formatDateUtil } from '../utils/date-format';
import { NAMESPACE } from '../../connect/tokens';
import opensElsewhere from '../opens-elsewhere';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/button';
import '@nldd/design-system/icon-cell';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/menu';
import '@nldd/design-system/modal-dialog';
import '@nldd/design-system/page';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/toolbar';
import '@nldd/design-system/top-title-bar';

@Component({
  selector: 'app-namespaces',
  imports: [ReactiveFormsModule, DialogSyncDirective, RouterOutlet],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './namespaces.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class NamespacesComponent implements OnInit {
  private titleService = inject(TitleService);

  protected pageNav = inject(PageNavService);

  protected overlays = inject(OverlayService);

  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private fb = inject(FormBuilder);

  private namespaceClient = inject(NAMESPACE);

  private idempotency = createIdempotencyRef();

  private notificationService = inject(NotificationService);

  private organizationDataService = inject(OrganizationDataService);

  projectId = signal<string>('');

  /** Named in the sheet title, so it says which project the namespace lands in;
   *  the page header is behind the sheet while it is open. */
  projectName = computed(() => {
    const gevonden = this.organizationDataService.getProjectById(this.projectId())?.project;
    return gevonden?.alias || gevonden?.name || 'this project';
  });

  namespaces = signal<Namespace[]>([]);

  /** True until the first load lands: an empty list and a list not yet loaded
   *  look the same, and only one of them should offer to create something. */
  isLoading = signal(true);

  errorMessage = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Namespaces');
  }

  async ngOnInit() {
    const projectId = this.route.snapshot.params['id'];
    this.projectId.set(projectId);
    await this.loadNamespaces(projectId);
  }

  /** The sheets that create and delete one stand over this very list without
   *  unmounting it, so the list hears about both from the shared signal. */
  private readonly reloadOnChange = effect(() => {
    this.organizationDataService.namespacesChanged();
    const projectId = this.projectId();
    if (projectId) this.loadNamespaces(projectId);
  });

  /** Routes client-side while leaving the row a real link, so middle-click and
   *  "open in new tab" keep working. */
  openNamespace(event: Event, name: string): void {
    if (opensElsewhere(event)) return;
    event.preventDefault();
    this.pageNav.goTo(`/projects/${this.projectId()}/namespaces/${name}`);
  }

  async loadNamespaces(projectId: string) {
    try {
      const request = create(ListProjectNamespacesRequestSchema, { projectId });
      const response = await firstValueFrom(this.namespaceClient.listProjectNamespaces(request));
      this.namespaces.set(response.namespaces);
    } catch (error) {
      this.notificationService.error(
        error instanceof Error
          ? `Failed to load namespaces: ${error.message}`
          : 'Failed to load namespaces',
      );
    } finally {
      this.isLoading.set(false);
    }
  }

  readonly formatDate = formatDateUtil;
}
