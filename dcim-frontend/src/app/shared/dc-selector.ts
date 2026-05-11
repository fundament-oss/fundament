import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { DATACENTER_INFO, DatacenterStatus } from '../datacenters/datacenter.model';

@Component({
  selector: 'app-dc-selector',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <nav class="flex h-10 items-center gap-0.5" aria-label="Datacenter selection">
      @for (dc of datacenters; track dc.id) {
        <button
          (click)="dcSelected.emit(dc.id)"
          class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors cursor-pointer"
          [class]="
            selectedId() === dc.id
              ? 'bg-slate-100 text-slate-900'
              : 'text-slate-500 hover:bg-slate-50 hover:text-slate-700'
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

  readonly datacenters = DATACENTER_INFO;

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
