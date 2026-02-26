import {
  Component,
  AfterViewInit,
  ViewChild,
  ViewContainerRef,
  ChangeDetectionStrategy,
  inject,
  signal,
} from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import PluginRegistryService from '../plugin-registry.service';
import PluginComponentRegistryService from '../plugin-component-registry.service';

@Component({
  selector: 'app-plugin-edit-dispatcher',
  standalone: true,
  template: `
    @if (loading()) {
      <div class="p-6 text-sm text-gray-500 dark:text-gray-400">Loading…</div>
    }
    <ng-template #outlet />
  `,
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class PluginEditDispatcherComponent implements AfterViewInit {
  @ViewChild('outlet', { read: ViewContainerRef }) private outlet!: ViewContainerRef;

  private route = inject(ActivatedRoute);

  private pluginRegistry = inject(PluginRegistryService);

  private componentRegistry = inject(PluginComponentRegistryService);

  loading = signal(true);

  async ngAfterViewInit(): Promise<void> {
    const params = this.route.snapshot.paramMap;
    const pluginName = params.get('pluginName') ?? '';
    const resourceKind = params.get('resourceKind') ?? '';

    const plugin = this.pluginRegistry.getPlugin(pluginName);
    const crd = this.pluginRegistry.getCrdByPlural(pluginName, resourceKind);
    const customName = crd ? plugin?.customComponents?.[crd.kind]?.edit : undefined;

    if (customName && this.componentRegistry.hasComponent(customName)) {
      const type = await this.componentRegistry.load(customName);
      if (type) {
        this.outlet.createComponent(type);
        this.loading.set(false);
        return;
      }
    }

    const { default: DefaultComponent } = await import('../resource-edit/resource-edit.component');
    this.outlet.createComponent(DefaultComponent);
    this.loading.set(false);
  }
}
