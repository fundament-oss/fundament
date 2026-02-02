import { Component, inject, OnInit, signal, computed } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { HttpClient } from '@angular/common/http';
import { TitleService } from '../title.service';
import { NgIconComponent, provideIcons } from '@ng-icons/core';
import { tablerPlugConnected, tablerRefresh, tablerRocket, tablerHome } from '@ng-icons/tabler-icons';
import { firstValueFrom } from 'rxjs';

interface PluginFeature {
  title: string;
  description: string;
}

interface PluginAction {
  id: string;
  label: string;
  icon: string;
  type: 'primary' | 'secondary';
  route?: string;
}

interface PluginAuthor {
  name: string;
  url: string;
}

interface PluginMetadata {
  loadedVia: string;
  endpoint: string;
  contentType: string;
  cacheable: boolean;
}

interface PluginConfig {
  id: string;
  name: string;
  version: string;
  description: string;
  author: PluginAuthor;
  status: 'active' | 'loading' | 'error';
  features: PluginFeature[];
  actions: PluginAction[];
  metadata: PluginMetadata;
}

interface PluginInfo extends PluginConfig {
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
      tablerHome,
    }),
  ],
})
export class RuntimePluginComponent implements OnInit {
  private titleService = inject(TitleService);
  private http = inject(HttpClient);

  // Signals for reactive state management
  isLoading = signal(true);
  error = signal<string | null>(null);
  pluginInfo = signal<PluginInfo | null>(null);
  loadCount = signal(0);
  xhrMethod = signal<string>('');

  // Computed signal based on plugin status
  statusBadgeClass = computed(() => {
    const status = this.pluginInfo()?.status;
    if (status === 'active') return 'badge-green';
    if (status === 'loading') return 'badge-blue';
    if (status === 'error') return 'badge-rose';
    return 'badge-gray';
  });

  constructor() {
    this.titleService.setTitle('Runtime Plugin Demo');
  }

  async ngOnInit() {
    await this.loadPlugin();
  }

  private async loadPlugin() {
    this.isLoading.set(true);
    this.error.set(null);
    const startTime = Date.now();

    try {
      // Load plugin configuration via XHR/HTTP request
      console.log('[Runtime Plugin] Fetching plugin configuration via XHR...');

      const config = await firstValueFrom(
        this.http.get<PluginConfig>('/plugins/runtime-plugin-config.json', {
          headers: {
            'Cache-Control': 'no-cache',
          },
        })
      );

      const loadTime = Date.now() - startTime;
      console.log(`[Runtime Plugin] Configuration loaded in ${loadTime}ms`);

      // Create plugin info with load metadata
      const pluginData: PluginInfo = {
        ...config,
        loadedAt: new Date(),
        loadTime,
      };

      this.pluginInfo.set(pluginData);
      this.loadCount.update((count) => count + 1);
      this.xhrMethod.set('XMLHttpRequest (XHR) via Angular HttpClient');
      this.isLoading.set(false);
    } catch (err) {
      console.error('[Runtime Plugin] Failed to load plugin configuration:', err);
      this.error.set(
        'Failed to load the runtime plugin configuration via XHR. Please ensure the server is running and try again.'
      );
      this.isLoading.set(false);
    }
  }

  async reloadPlugin() {
    await this.loadPlugin();
  }

  handleAction(action: PluginAction) {
    console.log(`[Runtime Plugin] Action triggered: ${action.id}`);
    if (action.id === 'refresh') {
      this.reloadPlugin();
    }
  }
}
