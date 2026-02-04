import { Component, inject, Input, OnInit, signal, ChangeDetectionStrategy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerChevronRight } from '@ng-icons/tabler-icons';
import { AUTHN, ORGANIZATION } from '../../connect/tokens';
import { firstValueFrom } from 'rxjs';
import { create } from '@bufbuild/protobuf';
import { GetOrganizationRequestSchema } from '../../generated/v1/organization_pb';

export interface BreadcrumbSegment {
  label: string;
  route?: string;
}

@Component({
  selector: 'app-breadcrumb',
  imports: [CommonModule, RouterLink, NgIcon],
  viewProviders: [
    provideIcons({
      tablerChevronRight,
    }),
  ],
  templateUrl: './breadcrumb.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class BreadcrumbComponent implements OnInit {
  private authnClient = inject(AUTHN);
  private organizationClient = inject(ORGANIZATION);

  // Support both the old API (currentPage) and new API (segments)
  @Input() currentPage?: string;
  @Input() segments: BreadcrumbSegment[] = [];

  organizationName = signal<string | null>(null);
  organizationLoading = signal(true);
  organizationError = signal<string | null>(null);

  async ngOnInit() {
    await this.loadOrganization();
  }

  // Computed segments that combines old and new API
  get allSegments(): BreadcrumbSegment[] {
    if (this.segments.length > 0) {
      return this.segments;
    }
    if (this.currentPage) {
      return [{ label: this.currentPage }];
    }
    return [];
  }

  async loadOrganization() {
    this.organizationLoading.set(true);
    this.organizationError.set(null);

    try {
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

      this.organizationName.set(response.organization.name);
    } catch (err) {
      console.error('Error loading organization:', err);
      this.organizationName.set(null);
      this.organizationError.set('Failed to load organization');
    } finally {
      this.organizationLoading.set(false);
    }
  }
}
