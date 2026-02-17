import { Component, input, output, ChangeDetectionStrategy } from '@angular/core';
import { NgIconComponent, provideIcons } from '@ng-icons/core';
import { tablerBuilding } from '@ng-icons/tabler-icons';
import type { Organization } from '../../generated/v1/organization_pb';

@Component({
  selector: 'app-org-picker',
  imports: [NgIconComponent],
  viewProviders: [provideIcons({ tablerBuilding })],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="flex min-h-[calc(100vh-4rem-1px)] items-center justify-center">
      <div class="w-full max-w-md">
        <div class="card">
          <div class="card-header">
            <h2 class="text-lg font-semibold dark:text-white">Select an organization</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              You belong to multiple organizations. Choose one to continue.
            </p>
          </div>
          <div class="card-body">
            <div class="space-y-1">
              @for (org of organizations(); track org.id) {
                <button
                  type="button"
                  (click)="onSelect(org.id)"
                  class="flex w-full cursor-pointer items-center rounded-md px-3 py-2.5 text-left text-sm font-medium text-gray-700 ring-indigo-500 transition-colors hover:bg-indigo-50 focus:ring-2 focus:ring-offset-2 focus:outline-none dark:text-gray-300 dark:ring-offset-gray-950 dark:hover:bg-indigo-950"
                >
                  <ng-icon name="tablerBuilding" class="mr-3 shrink-0" size="1.25rem" />
                  <span class="truncate">{{ org.name }}</span>
                </button>
              }
            </div>
          </div>
        </div>
      </div>
    </div>
  `,
})
export default class OrgPickerComponent {
  organizations = input<Organization[]>([]);

  selectOrganization = output<string>();

  onSelect(orgId: string) {
    this.selectOrganization.emit(orgId);
  }
}
