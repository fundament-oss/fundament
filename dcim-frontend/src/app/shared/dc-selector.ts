import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { DatacenterInfo, DatacenterStatus } from '../datacenters/datacenter.model';

@Component({
  selector: 'app-dc-selector',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <nav class="flex h-10 items-center gap-0.5" aria-label="Datacenter selection">
      @for (dc of datacenters(); track dc.id) {
        <button
          (click)="dcSelected.emit(dc.id)"
          class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors cursor-pointer"
          [class]="
            selectedId() === dc.id
              ? 'bg-slate-100 dark:bg-gray-800 text-slate-900 dark:text-white'
              : 'text-slate-500 dark:text-gray-400 hover:bg-slate-50 dark:hover:bg-gray-900 hover:text-slate-700 dark:hover:text-gray-300'
          "
          [attr.aria-pressed]="selectedId() === dc.id"
        >
          {{ dc.name }}
          <span
            class="h-1.5 w-1.5 rounded-full shrink-0"
            [class]="statusDotClass(dc.status)"
            aria-hidden="true"
          ></span>
        </button>
      }
    </nav>
  `,
})
export default class DcSelectorComponent {
  readonly selectedId = input.required<string>();

  readonly dcSelected = output<string>();

  readonly datacenters = input.required<DatacenterInfo[]>();

  readonly statusDotClass = (status: DatacenterStatus): string => {
    switch (status) {
      case 'operational':
        return 'bg-teal-500';
      case 'degraded':
        return 'bg-amber-500';
      case 'maintenance':
        return 'bg-slate-400';
      default:
        return '';
    }
  };
}
