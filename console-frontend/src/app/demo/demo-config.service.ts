// Demo-only stand-in for ConfigService: returns fixed dummy URLs without fetching.
// The demo transports are in-memory (createRouterTransport ignores baseUrl), so these
// values are never used for real network calls.
import { AppConfiguration } from '../config.service';

export class DemoConfigService {
  constructor(private readonly config: AppConfiguration) {}

  async loadConfig(): Promise<AppConfiguration> {
    return this.config;
  }

  getConfig(): AppConfiguration {
    return this.config;
  }
}
