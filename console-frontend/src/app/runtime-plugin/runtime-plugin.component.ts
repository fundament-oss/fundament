import {
  Component,
  inject,
  AfterViewInit,
  signal,
  ViewChild,
  ViewContainerRef,
  ComponentRef,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { NgIconComponent, provideIcons } from '@ng-icons/core';
import { tablerPlugConnected, tablerRefresh, tablerRocket, tablerCode } from '@ng-icons/tabler-icons';
import {
  DynamicComponentLoaderService,
  RemoteComponentDefinition,
} from '../dynamic-component-loader.service';

interface LoadedComponentInfo {
  definition: RemoteComponentDefinition;
  componentRef: ComponentRef<unknown>;
  loadedAt: Date;
  loadTime: number;
}

@Component({
  selector: 'app-runtime-plugin',
  standalone: true,
  imports: [CommonModule, RouterLink, NgIconComponent],
  templateUrl: './runtime-plugin.component.html',
  viewProviders: [
    provideIcons({
      tablerPlugConnected,
      tablerRefresh,
      tablerRocket,
      tablerCode,
    }),
  ],
})
export class RuntimePluginComponent implements AfterViewInit {
  private titleService = inject(TitleService);
  private dynamicLoader = inject(DynamicComponentLoaderService);

  @ViewChild('dynamicComponentContainer', { read: ViewContainerRef })
  container!: ViewContainerRef;

  isLoading = signal(true);
  error = signal<string | null>(null);
  componentInfo = signal<LoadedComponentInfo | null>(null);
  loadCount = signal(0);

  constructor() {
    this.titleService.setTitle('Dynamic Component Loading');
  }

  async ngAfterViewInit() {
    await this.loadDynamicComponent();
  }

  private async loadDynamicComponent() {
    this.isLoading.set(true);
    this.error.set(null);
    const startTime = Date.now();

    try {
      this.container.clear();

      // Fetch component definition via XHR
      const definition = await this.dynamicLoader.fetchComponentDefinition(
        '/plugins/demo-widget.json'
      );

      // Compile the component at runtime
      const componentType = await this.dynamicLoader.compileComponent(definition);

      // Create and render the component
      const componentRef = this.container.createComponent(componentType);
      const loadTime = Date.now() - startTime;

      this.componentInfo.set({
        definition,
        componentRef,
        loadedAt: new Date(),
        loadTime,
      });

      this.loadCount.update((count) => count + 1);
      this.isLoading.set(false);
    } catch (err) {
      console.error('[Runtime Plugin] Failed to load component:', err);
      this.error.set(
        `Failed to load and compile the dynamic component. Error: ${err instanceof Error ? err.message : 'Unknown error'}`
      );
      this.isLoading.set(false);
    }
  }

  async reloadComponent() {
    await this.loadDynamicComponent();
  }

  get componentDefinition() {
    return this.componentInfo()?.definition;
  }
}
