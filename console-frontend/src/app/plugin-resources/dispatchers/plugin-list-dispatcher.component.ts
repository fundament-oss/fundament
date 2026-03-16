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
import PluginPermissionService from '../plugin-permission.service';

/**
 * Dispatcher for the resource list view.
 * Checks if the plugin has a custom list component registered; falls back to ResourceListComponent.
 * Resolves canWrite from PluginPermissionService and passes it to the loaded component.
 */
@Component({
  selector: 'app-plugin-list-dispatcher',
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

  private permissionService = inject(PluginPermissionService);

  private destroyRef = inject(DestroyRef);

  loading = signal(true);

  ngAfterViewInit(): void {
    this.route.paramMap.pipe(takeUntilDestroyed(this.destroyRef)).subscribe(async (params) => {
      const pluginName = params.get('pluginName') ?? '';
      const resourceKind = params.get('resourceKind') ?? '';
      const projectId = params.get('id') ?? undefined;

      this.outlet.clear();
      this.loading.set(true);

      await this.permissionService.loadOrgPermission();
      const canWrite = projectId
        ? await this.permissionService.canWriteProject(projectId)
        : this.permissionService.isOrgAdmin();

      const plugin = this.pluginRegistry.getPlugin(pluginName);
      const crd = this.pluginRegistry.getCrdByPlural(pluginName, resourceKind);
      const customName = crd ? plugin?.customComponents?.[crd.kind]?.list : undefined;

      if (customName && this.componentRegistry.hasComponent(customName)) {
        const type = await this.componentRegistry.load(customName);
        if (type) {
          const ref = this.outlet.createComponent(type);
          ref.setInput('canWrite', canWrite);
          this.loading.set(false);
          return;
        }
      }

      const { default: DefaultComponent } = await import(
        '../resource-list/resource-list.component'
      );
      const ref = this.outlet.createComponent(DefaultComponent);
      ref.setInput('canWrite', canWrite);
      this.loading.set(false);
    });
  }
}
