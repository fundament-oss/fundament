// Demo-only stand-in for ConfigService: returns a fixed configuration without fetching.
// The demo transports are in-memory (createRouterTransport ignores baseUrl), so the API
// URLs are never used for real network calls. The link-valued entries are real, though —
// see demo-app.config.ts.
import { AppConfiguration, ConfigService } from '../config.service';

export default class DemoConfigService implements Pick<ConfigService, 'loadConfig' | 'getConfig'> {
  constructor(private readonly config: AppConfiguration) {}

  async loadConfig(): Promise<AppConfiguration> {
    return this.config;
  }

  getConfig(): AppConfiguration {
    return this.config;
  }
}
