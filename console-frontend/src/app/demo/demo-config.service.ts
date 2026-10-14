// Demo-only stand-in for ConfigService: returns a fixed configuration. The demo
// transports are in-memory (createRouterTransport ignores baseUrl), so the API
// URLs are never used for real network calls. The link-valued entries are real,
// though — see demo-app.config.ts.
import { AppConfiguration, ConfigService } from '../config.service';

export default class DemoConfigService implements Pick<ConfigService, 'loadConfig' | 'getConfig'> {
  constructor(private config: AppConfiguration) {}

  /**
   * Takes one thing from the deployment's config.json: pocUrl, so a walkthrough
   * deployed to a test environment carries the same banner as the console there.
   * Everything else stays fixed. A file that is missing or unreadable (a static
   * host, `bun start`) leaves the fixed configuration as it is.
   */
  async loadConfig(): Promise<AppConfiguration> {
    try {
      const response = await fetch('/assets/config/config.json');
      if (response.ok) {
        const { pocUrl } = (await response.json()) as Partial<AppConfiguration>;
        if (pocUrl) this.config = { ...this.config, pocUrl };
      }
    } catch {
      // No deployment config: the walkthrough runs without the banner.
    }
    return this.config;
  }

  getConfig(): AppConfiguration {
    return this.config;
  }
}
