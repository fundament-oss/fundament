import {
  Component,
  ChangeDetectionStrategy,
  inject,
  signal,
  OnInit,
  ViewContainerRef,
  viewChildren,
  AfterViewInit,
  ChangeDetectorRef,
} from '@angular/core';
import PluginRegistryService from '../plugin-resources/plugin-registry.service';
import PluginComponentRegistryService from '../plugin-resources/plugin-component-registry.service';
import KubeClusterContextService from '../plugin-resources/kube-cluster-context.service';
import { TitleService } from '../title.service';
import type { WidgetDefinition } from '../plugin-resources/types';

interface ResolvedWidget {
  definition: WidgetDefinition;
  pluginName: string;
}

function widgetClass(size: 'small' | 'medium' | 'large'): string {
  switch (size) {
    case 'small':
      return 'col-span-1';
    case 'large':
      return 'col-span-1 sm:col-span-2 lg:col-span-3';
    default:
      return 'col-span-1 sm:col-span-2';
  }
}

@Component({
  selector: 'app-cluster-dashboard',
  template: `
    <div class="space-y-6">
      <div class="card px-6 py-4">
        <h1 class="text-2xl font-bold dark:text-white">Dashboard</h1>
        <p class="mt-1 text-sm text-gray-600 dark:text-gray-300">
          Plugin widgets for the selected cluster.
        </p>
      </div>

      @if (widgets().length === 0) {
        <div class="card">
          <div class="card-body text-center text-gray-500 dark:text-gray-400">
            No dashboard widgets configured. Plugin authors can declare
            <code class="font-mono text-sm">dashboardWidgets</code> in their plugin manifest.
          </div>
        </div>
      } @else {
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          @for (widget of widgets(); track widget.definition.id) {
            <div [class]="widgetClass(widget.definition.size)" class="card overflow-hidden">
              <div class="card-header">
                <h2 class="text-sm font-semibold dark:text-white">{{ widget.definition.title }}</h2>
              </div>
              <div class="card-body p-0">
                <ng-container #widgetOutlet />
              </div>
            </div>
          }
        </div>
      }
    </div>
  `,
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ClusterDashboardComponent implements OnInit, AfterViewInit {
  private registry = inject(PluginRegistryService);

  private componentRegistry = inject(PluginComponentRegistryService);

  private clusterContext = inject(KubeClusterContextService);

  private titleService = inject(TitleService);

  private cdr = inject(ChangeDetectorRef);

  widgets = signal<ResolvedWidget[]>([]);

  widgetOutlets = viewChildren('widgetOutlet', { read: ViewContainerRef });

  constructor() {
    this.titleService.setTitle('Dashboard');
  }

  async ngOnInit(): Promise<void> {
    await this.clusterContext.loadClusters();

    const allWidgets: ResolvedWidget[] = this.registry
      .allPlugins()
      .filter((plugin) => plugin.dashboardWidgets)
      .flatMap((plugin) =>
        plugin.dashboardWidgets!.map((widget) => ({ definition: widget, pluginName: plugin.name })),
      );

    this.widgets.set(allWidgets);
    this.cdr.detectChanges();
  }

  async ngAfterViewInit(): Promise<void> {
    const outlets = this.widgetOutlets();
    const widgetList = this.widgets();

    await Promise.all(
      widgetList.map(async (widget, i) => {
        const outlet = outlets[i];
        if (!outlet) return;
        const type = await this.componentRegistry.load(widget.definition.component);
        if (type) {
          outlet.createComponent(type);
        }
      }),
    );
  }

  protected readonly widgetClass = widgetClass;
}
