import { Injectable, InjectionToken, inject } from '@angular/core';

// Runtime configuration, fetched before the app bootstraps. The three
// marketplace APIs are three deployables (FUN-20), so each gets its own base
// URL rather than a shared one with a path prefix.
export interface AppConfiguration {
  catalogApiUrl: string;
  registryApiUrl: string;
  adminApiUrl: string;
  /**
   * Public URL of the console. Optional: the storefront has no cluster context
   * of its own, so a listing's "Install plugin" button hands the visitor over
   * to the console. Empty means no console is reachable from this environment.
   */
  consoleUrl?: string;
}

export type ConfigLoader = () => Promise<AppConfiguration>;

export const EMPTY_CONFIGURATION: AppConfiguration = {
  catalogApiUrl: '',
  registryApiUrl: '',
  adminApiUrl: '',
};

async function fetchConfig(): Promise<AppConfiguration> {
  const response = await fetch('/assets/config/config.json');
  if (!response.ok) {
    throw new Error(`Failed to load config: ${response.statusText}`);
  }
  return (await response.json()) as AppConfiguration;
}

/**
 * How `ConfigService` obtains the runtime configuration. The browser fetches
 * the file the deployment mounts next to the app; the server bundle overrides
 * this with a loader that reads the configuration out of the request context
 * (see `app.config.server.ts`), because a relative fetch has no origin there.
 */
export const CONFIG_LOADER = new InjectionToken<ConfigLoader>('marketplace-config-loader', {
  providedIn: 'root',
  factory: () => fetchConfig,
});

@Injectable({
  providedIn: 'root',
})
export class ConfigService {
  private config?: AppConfiguration;

  private loader = inject(CONFIG_LOADER);

  async loadConfig(): Promise<AppConfiguration> {
    if (this.config) {
      return this.config;
    }

    try {
      this.config = await this.loader();
      return this.config;
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to load configuration', error);

      this.config = EMPTY_CONFIGURATION;

      return this.config;
    }
  }

  getConfig(): AppConfiguration {
    if (!this.config) {
      throw new Error('Configuration not loaded. Call loadConfig() first.');
    }
    return this.config;
  }
}
