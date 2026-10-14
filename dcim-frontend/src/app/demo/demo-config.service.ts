import { Injectable } from '@angular/core';
import { AppConfiguration } from '../config.service';

/** No config to fetch: every call goes to the in-memory transports, so no url
 *  is ever used. The platform one is set only so the Platform section knows it
 *  has an API to read. */
@Injectable()
export default class DemoConfigService {
  private readonly config: AppConfiguration = {
    authnApiUrl: '',
    apiUrl: '',
    platformApiUrl: 'demo://platform',
  };

  async loadConfig(): Promise<AppConfiguration> {
    return this.config;
  }

  getConfig(): AppConfiguration {
    return this.config;
  }
}
