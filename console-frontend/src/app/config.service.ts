import { Injectable } from '@angular/core';

export interface AppConfiguration {
  authnApiUrl: string;
  organizationApiUrl: string;
  kubeApiProxyUrl: string;
  pluginProxyUrl: string;
  /**
   * Public URL of the walkthrough (console-demo subdomain). Optional: the demo
   * build is not deployed in every environment, and the header's "Take a tour"
   * button is hidden when this is empty.
   */
  consoleDemoUrl?: string;
  /**
   * Public URL of the plugin marketplace. Optional: the marketplace is not
   * deployed in every environment, and the plugin sheet's "View full details"
   * button falls back to the console's own plugin page when this is empty.
   */
  marketplaceUrl?: string;
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
        authnApiUrl: '',
        organizationApiUrl: '',
        kubeApiProxyUrl: '',
        pluginProxyUrl: '',
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
