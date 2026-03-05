import {
  Component,
  AfterViewInit,
  ViewChild,
  ViewContainerRef,
  ChangeDetectionStrategy,
  signal,
} from '@angular/core';

@Component({
  selector: 'app-plugin-detail-dispatcher',
  standalone: true,
  template: `
    @if (loading()) {
      <div class="p-6 text-sm text-gray-500 dark:text-gray-400">Loading…</div>
    }
    <ng-template #outlet />
  `,
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class PluginDetailDispatcherComponent implements AfterViewInit {
  @ViewChild('outlet', { read: ViewContainerRef }) private outlet!: ViewContainerRef;

  loading = signal(true);

  async ngAfterViewInit(): Promise<void> {
    const { default: DefaultComponent } = await import(
      '../resource-detail/resource-detail.component'
    );
    this.outlet.createComponent(DefaultComponent);
    this.loading.set(false);
  }
}
