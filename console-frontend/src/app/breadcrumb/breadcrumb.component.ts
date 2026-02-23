import { Component, computed, inject, Input, ChangeDetectionStrategy } from '@angular/core';
import { RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerChevronRight } from '@ng-icons/tabler-icons';
import { OrganizationDataService } from '../organization-data.service';

export interface BreadcrumbSegment {
  label: string;
  route?: string;
}

@Component({
  selector: 'app-breadcrumb',
  imports: [RouterLink, NgIcon],
  viewProviders: [
    provideIcons({
      tablerChevronRight,
    }),
  ],
  templateUrl: './breadcrumb.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class BreadcrumbComponent {
  private orgDataService = inject(OrganizationDataService);

  // Support both the old API (currentPage) and new API (segments)
  @Input() currentPage?: string;

  @Input() segments: BreadcrumbSegment[] = [];

  organizationName = computed(() => this.orgDataService.organizations()[0]?.name ?? null);

  organizationLoading = this.orgDataService.loading;

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
}
