import { Component, inject, OnInit, ViewChild, ElementRef, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { TitleService } from '../title.service';
import { AUTHN, ORGANIZATION } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import {
  GetOrganizationRequestSchema,
  UpdateOrganizationRequestSchema,
  Organization,
} from '../../generated/v1/organization_pb';
import { firstValueFrom } from 'rxjs';
import { CheckmarkIconComponent, CloseIconComponent, EditIconComponent } from '../icons';

@Component({
  selector: 'app-organization',
  standalone: true,
  imports: [
    CommonModule,
    FormsModule,
    CheckmarkIconComponent,
    CloseIconComponent,
    EditIconComponent,
  ],
  templateUrl: './organization.component.html',
})
export class OrganizationComponent implements OnInit {
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
      this.error.set(err instanceof Error ? err.message : 'Failed to load organization');
      console.error('Error loading organization:', err);
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

      const response = await firstValueFrom(this.organizationClient.updateOrganization(request));

      if (!response.organization) {
        throw new Error('Failed to update organization');
      }

      this.organization.set(response.organization);
      this.isEditing.set(false);
      this.editingName.set('');
    } catch (err) {
      this.error.set(err instanceof Error ? err.message : 'Failed to update organization');
      console.error('Error updating organization:', err);
    } finally {
      this.loading.set(false);
    }
  }

  formatDate(dateString: string): string {
    try {
      return new Date(dateString).toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
      });
    } catch {
      return dateString;
    }
  }
}
