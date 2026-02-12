import {
  Component,
  inject,
  OnInit,
  ViewChild,
  ElementRef,
  signal,
  ChangeDetectionStrategy,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerPencil, tablerX, tablerCheck } from '@ng-icons/tabler-icons';
import {
  GetOrganizationRequestSchema,
  UpdateOrganizationRequestSchema,
  Organization,
} from '../../generated/v1/organization_pb';
import { ORGANIZATION } from '../../connect/tokens';
import { TitleService } from '../title.service';
import { OrganizationDataService } from '../organization-data.service';
import { formatDate as formatDateUtil } from '../utils/date-format';
import { OrganizationContextService } from '../organization-context.service';

@Component({
  selector: 'app-organization',
  imports: [FormsModule, NgIcon],
  viewProviders: [
    provideIcons({
      tablerPencil,
      tablerX,
      tablerCheck,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './organization.component.html',
})
export default class OrganizationComponent implements OnInit {
  private titleService = inject(TitleService);

  private organizationClient = inject(ORGANIZATION);

  private organizationContextService = inject(OrganizationContextService);

  private organizationDataService = inject(OrganizationDataService);

  @ViewChild('nameInput') nameInput?: ElementRef<HTMLInputElement>;

  organization = signal<Organization | null>(null);

  isEditing = signal(false);

  editingName = signal('');

  loading = signal(false);

  error = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Organization settings');
  }

  async ngOnInit() {
    await this.loadOrganization();
  }

  async loadOrganization() {
    this.loading.set(true);
    this.error.set(null);

    try {
      // Get organization ID from context service
      const organizationId = this.organizationContextService.currentOrganizationId();
      if (!organizationId) {
        throw new Error('Organization ID not found');
      }

      const request = create(GetOrganizationRequestSchema, {
        id: organizationId,
      });
      const response = await firstValueFrom(this.organizationClient.getOrganization(request));

      if (!response.organization) {
        throw new Error('Organization not found');
      }

      this.organization.set(response.organization);
    } catch (err) {
      this.error.set(
        err instanceof Error
          ? `Failed to load organization: ${err.message}`
          : 'Failed to load organization',
      );
    } finally {
      this.loading.set(false);
    }
  }

  startEdit() {
    const currentOrganization = this.organization();
    if (currentOrganization) {
      this.isEditing.set(true);
      this.editingName.set(currentOrganization.name);

      // Focus the input field after Angular updates the view
      setTimeout(() => {
        this.nameInput?.nativeElement.focus();
      });
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
        name: nameToSave.trim(),
      });

      await firstValueFrom(this.organizationClient.updateOrganization(request));

      // Update the local organization with the new name
      this.organization.set({
        ...currentOrganization,
        name: nameToSave.trim(),
      });
      this.organizationDataService.updateOrganizationName(
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
