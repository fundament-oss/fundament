import { Injectable } from '@angular/core';

export interface AppConfiguration {
  authnApiUrl: string;
  apiUrl: string;
  /** The staff fleet API (fleet.v1, FUN-23). Optional: the Platform section
   *  says it is not available when this is empty. */
  platformApiUrl?: string;
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
        apiUrl: '',
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
