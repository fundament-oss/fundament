import {
  Component,
  AfterViewInit,
  ViewChild,
  ViewContainerRef,
  ChangeDetectionStrategy,
  inject,
  signal,
  DestroyRef,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute } from '@angular/router';
import PluginRegistryService from '../plugin-registry.service';
import PluginComponentRegistryService from '../plugin-component-registry.service';

/**
 * Dispatcher for the plugin list view.
 *
 * If the plugin's `customComponents` section names a list component for the current CRD kind,
 * that component is lazily loaded and rendered. Otherwise the default ResourceListComponent
 * is used. Both inherit the current injector context (including ActivatedRoute).
 */
@Component({
  selector: 'app-plugin-list-dispatcher',
  standalone: true,
  template: `
    @if (loading()) {
      <div class="p-6 text-sm text-gray-500 dark:text-gray-400">Loading…</div>
    }
    <ng-template #outlet />
  `,
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class PluginListDispatcherComponent implements AfterViewInit {
  @ViewChild('outlet', { read: ViewContainerRef }) private outlet!: ViewContainerRef;

  private route = inject(ActivatedRoute);

  private pluginRegistry = inject(PluginRegistryService);

  private componentRegistry = inject(PluginComponentRegistryService);

  private destroyRef = inject(DestroyRef);

  loading = signal(true);

  ngAfterViewInit(): void {
    this.route.paramMap.pipe(takeUntilDestroyed(this.destroyRef)).subscribe(async (params) => {
      const pluginName = params.get('pluginName') ?? '';
      const resourceKind = params.get('resourceKind') ?? '';

      this.outlet.clear();
      this.loading.set(true);

      const plugin = this.pluginRegistry.getPlugin(pluginName);
      const crd = this.pluginRegistry.getCrdByPlural(pluginName, resourceKind);
      const customName = crd ? plugin?.customComponents?.[crd.kind]?.list : undefined;

      if (customName && this.componentRegistry.hasComponent(customName)) {
        const type = await this.componentRegistry.load(customName);
        if (type) {
          this.outlet.createComponent(type);
          this.loading.set(false);
          return;
        }
      }

      const { default: DefaultComponent } =
        await import('../resource-list/resource-list.component');
      this.outlet.createComponent(DefaultComponent);
      this.loading.set(false);
    });
  }
}
