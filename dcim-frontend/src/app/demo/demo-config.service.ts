import { Injectable } from '@angular/core';
import { AppConfiguration } from '../config.service';

/** No config to fetch: every call goes to the in-memory transport, so neither
 *  url is ever used. */
@Injectable()
export default class DemoConfigService {
  private readonly config: AppConfiguration = { authnApiUrl: '', apiUrl: '' };

  async loadConfig(): Promise<AppConfiguration> {
    return this.config;
  }

  getConfig(): AppConfiguration {
    return this.config;
  }
}
