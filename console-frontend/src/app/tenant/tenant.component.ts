import { Component, inject, OnInit, ChangeDetectorRef, ViewChild, ElementRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Title } from '@angular/platform-browser';
import { FormsModule } from '@angular/forms';
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
  private titleService = inject(Title);
  private apiService = inject(ApiService);
  private organizationApiService = inject(OrganizationApiService);
  private cdr = inject(ChangeDetectorRef);

  @ViewChild('nameInput') nameInput?: ElementRef<HTMLInputElement>;

  tenant: Tenant | null = null;
  isEditing = false;
  editingName = '';
  loading = false;
  error: string | null = null;

  constructor() {
    this.titleService.setTitle('Tenant — Fundament Console');
  }

  async ngOnInit() {
    await this.loadTenant();
  }

  async loadTenant() {
    this.loading = true;
    this.error = null;

    try {
      // Get current user to retrieve tenant ID
      const userInfo = await this.apiService.getUserInfo();
      this.tenant = await this.organizationApiService.getTenant(userInfo.tenantId);
    } catch (err) {
      this.error = err instanceof Error ? err.message : 'Failed to load tenant';
      console.error('Error loading tenant:', err);
    } finally {
      this.loading = false;
      this.cdr.detectChanges();
    }
  }

  startEdit() {
    if (this.tenant) {
      this.isEditing = true;
      this.editingName = this.tenant.name;

      // Focus the input field after Angular updates the view
      setTimeout(() => {
        this.nameInput?.nativeElement.focus();
      });
    }
  }

  cancelEdit() {
    this.isEditing = false;
    this.editingName = '';
  }

  async saveEdit() {
    if (!this.editingName.trim() || !this.tenant) {
      return;
    }

    this.loading = true;
    this.error = null;

    try {
      this.tenant = await this.organizationApiService.updateTenant(
        this.tenant.id,
        this.editingName.trim(),
      );
      this.isEditing = false;
      this.editingName = '';
    } catch (err) {
      this.error = err instanceof Error ? err.message : 'Failed to update tenant';
      console.error('Error updating tenant:', err);
    } finally {
      this.loading = false;
      this.cdr.detectChanges();
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
