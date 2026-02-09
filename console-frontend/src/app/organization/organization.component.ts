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
import { AUTHN, ORGANIZATION } from '../../connect/tokens';
import { TitleService } from '../title.service';
import { formatDate as formatDateUtil } from '../utils/date-format';

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

  private authnClient = inject(AUTHN);

  private organizationClient = inject(ORGANIZATION);

  @ViewChild('nameInput') nameInput?: ElementRef<HTMLInputElement>;

  organization = signal<Organization | null>(null);

  isEditing = signal(false);

  editingName = signal('');

  loading = signal(false);

  error = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Organization details');
  }

  async ngOnInit() {
    await this.loadOrganization();
  }

  async loadOrganization() {
    this.loading.set(true);
    this.error.set(null);

    try {
      // Get current user to retrieve organization ID
      const userResponse = await firstValueFrom(this.authnClient.getUserInfo({}));
      if (!userResponse.user?.organizationId) {
        throw new Error('Organization ID not found');
      }

      const request = create(GetOrganizationRequestSchema, {
        id: userResponse.user.organizationId,
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
