import {
  Component,
  inject,
  OnInit,
  signal,
  computed,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import AutofocusDirective from '../autofocus.directive';
import SheetSyncDirective from '../sheet-sync.directive';
import PageNavService from '../page-nav.service';
import { UpdateOrganizationRequestSchema } from '../../generated/v1/organization_pb';
import { ORGANIZATION } from '../../connect/tokens';
import { TitleService } from '../title.service';
import { OrganizationDataService, type OrganizationData } from '../organization-data.service';
import { formatDate as formatDateUtil } from '../utils/date-format';
import OrganizationContextService from '../organization-context.service';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/banner';
import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/cell';
import '@nldd/design-system/container';
import '@nldd/design-system/form';
import '@nldd/design-system/form-actions';
import '@nldd/design-system/form-field';
import '@nldd/design-system/icon-button';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/text-field';
import '@nldd/design-system/title';
import '@nldd/design-system/top-title-bar';
import '@nldd/design-system/validation-list';

@Component({
  selector: 'app-organization-settings',
  imports: [AutofocusDirective, SheetSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './organization-settings.component.html',
})
export default class OrganizationComponent implements OnInit {
  private titleService = inject(TitleService);

  protected pageNav = inject(PageNavService);

  private organizationClient = inject(ORGANIZATION);

  private organizationContextService = inject(OrganizationContextService);

  private organizationDataService = inject(OrganizationDataService);

  organization = signal<OrganizationData | null>(null);

  isEditing = signal(false);

  editingName = signal('');

  loading = signal(false);

  error = signal<string | null>(null);

  /** Set by pressing Save on an empty field: the button is never disabled, so
   *  the field is what says what is missing. */
  private saveAttempted = signal(false);

  aliasInvalid = computed(() => this.saveAttempted() && this.editingName().trim() === '');

  constructor() {
    this.titleService.setTitle('General');
  }

  ngOnInit() {
    const orgId = this.organizationContextService.currentOrganizationId();
    const orgData = orgId ? this.organizationDataService.getOrganizationById(orgId) : null;
    this.organization.set(orgData ?? null);
  }

  startEdit() {
    const currentOrganization = this.organization();
    if (currentOrganization) {
      this.saveAttempted.set(false);
      this.error.set(null);
      this.editingName.set(currentOrganization.alias);
      this.isEditing.set(true);
    }
  }

  cancelEdit() {
    this.isEditing.set(false);
    this.editingName.set('');
  }

  async saveEdit() {
    const currentOrganization = this.organization();
    const nameToSave = this.editingName();

    this.saveAttempted.set(true);
    if (!nameToSave.trim() || !currentOrganization) {
      return;
    }

    this.loading.set(true);
    this.error.set(null);

    try {
      const request = create(UpdateOrganizationRequestSchema, {
        id: currentOrganization.id,
        alias: nameToSave.trim(),
      });

      await firstValueFrom(this.organizationClient.updateOrganization(request));

      this.organization.set({ ...currentOrganization, alias: nameToSave.trim() });
      this.organizationDataService.updateOrganizationAlias(
        currentOrganization.id,
        nameToSave.trim(),
      );
      this.isEditing.set(false);
      this.editingName.set('');
    } catch (err) {
      // The sheet stays open: this is where the change is.

      this.error.set(err instanceof Error ? err.message : 'The request failed.');
    } finally {
      this.loading.set(false);
    }
  }

  readonly formatDate = formatDateUtil;
}
