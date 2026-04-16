import {
  Component,
  inject,
  OnInit,
  signal,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import AutofocusDirective from '../autofocus.directive';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { UpdateOrganizationRequestSchema } from '../../generated/v1/organization_pb';
import { ORGANIZATION } from '../../connect/tokens';
import { TitleService } from '../title.service';
import { OrganizationDataService, type OrganizationData } from '../organization-data.service';
import { formatDate as formatDateUtil } from '../utils/date-format';
import OrganizationContextService from '../organization-context.service';

@Component({
  selector: 'app-organization-settings',
  imports: [FormsModule, AutofocusDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './organization-settings.component.html',
})
export default class OrganizationComponent implements OnInit {
  private titleService = inject(TitleService);

  private organizationClient = inject(ORGANIZATION);

  private organizationContextService = inject(OrganizationContextService);

  private organizationDataService = inject(OrganizationDataService);

  organization = signal<OrganizationData | null>(null);

  isEditing = signal(false);

  editingName = signal('');

  loading = signal(false);

  error = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Organization settings');
  }

  ngOnInit() {
    const orgId = this.organizationContextService.currentOrganizationId();
    const orgData = orgId ? this.organizationDataService.getOrganizationById(orgId) : null;
    this.organization.set(orgData ?? null);
  }

  startEdit() {
    const currentOrganization = this.organization();
    if (currentOrganization) {
      this.isEditing.set(true);
      this.editingName.set(currentOrganization.alias);

    }
  }

  cancelEdit() {
    this.isEditing.set(false);
    this.editingName.set('');
  }

  async saveEdit() {
    const currentOrganization = this.organization();
    const nameToSave = this.editingName();

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
      this.error.set(
        err instanceof Error
          ? `Failed to update organization: ${err.message}`
          : 'Failed to update organization',
      );
    } finally {
      this.loading.set(false);
    }
  }

  readonly formatDate = formatDateUtil;
}
