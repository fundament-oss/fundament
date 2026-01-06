import { Component, inject, OnInit, ViewChild, ElementRef, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { TitleService } from '../title.service';
import { ApiService } from '../api.service';
import { OrganizationApiService, Tenant } from '../organization-api.service';
import { CheckmarkIconComponent, CloseIconComponent, EditIconComponent } from '../icons';

@Component({
  selector: 'app-tenant',
  standalone: true,
  imports: [
    CommonModule,
    FormsModule,
    CheckmarkIconComponent,
    CloseIconComponent,
    EditIconComponent,
  ],
  templateUrl: './tenant.component.html',
})
export class TenantComponent implements OnInit {
  private titleService = inject(TitleService);
  private apiService = inject(ApiService);
  private organizationApiService = inject(OrganizationApiService);

  @ViewChild('nameInput') nameInput?: ElementRef<HTMLInputElement>;

  tenant = signal<Tenant | null>(null);
  isEditing = signal(false);
  editingName = signal('');
  loading = signal(false);
  error = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Tenant details');
  }

  async ngOnInit() {
    await this.loadTenant();
  }

  async loadTenant() {
    this.loading.set(true);
    this.error.set(null);

    try {
      // Get current user to retrieve tenant ID
      const userInfo = await this.apiService.getUserInfo();
      this.tenant.set(await this.organizationApiService.getTenant(userInfo.tenantId));
    } catch (err) {
      this.error.set(err instanceof Error ? err.message : 'Failed to load tenant');
      console.error('Error loading tenant:', err);
    } finally {
      this.loading.set(false);
    }
  }

  startEdit() {
    const currentTenant = this.tenant();
    if (currentTenant) {
      this.isEditing.set(true);
      this.editingName.set(currentTenant.name);

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
    const currentTenant = this.tenant();
    const nameToSave = this.editingName();
    
    if (!nameToSave.trim() || !currentTenant) {
      return;
    }

    this.loading.set(true);
    this.error.set(null);

    try {
      this.tenant.set(await this.organizationApiService.updateTenant(
        currentTenant.id,
        nameToSave.trim(),
      ));
      this.isEditing.set(false);
      this.editingName.set('');
    } catch (err) {
      this.error.set(err instanceof Error ? err.message : 'Failed to update tenant');
      console.error('Error updating tenant:', err);
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
