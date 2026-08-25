import { Injectable } from '@angular/core';

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

@Injectable({
  providedIn: 'root',
})
export class ConfigService {
  private config?: AppConfiguration;

  async loadConfig(): Promise<AppConfiguration> {
    if (this.config) {
      return this.config;
    }

    try {
      const response = await fetch('/assets/config/config.json');
      if (!response.ok) {
        throw new Error(`Failed to load config: ${response.statusText}`);
      }
      this.config = await response.json();
      return this.config!;
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to load configuration', error);

      this.config = {
        catalogApiUrl: '',
        registryApiUrl: '',
        adminApiUrl: '',
      };

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
