import { inject } from '@angular/core';
import { ConfigService } from '../config.service';
import { PRESENTATION_ENABLED } from '../presentation/presentation.tokens';

/**
 * Where plugins are found: the marketplace, or '' when the console's own
 * plugins page has to go on offering the whole catalog.
 *
 * With a marketplace deployed, discovering plugins is its job, and the
 * console's page lists only what is installed. Without one that page is the
 * only catalog there is. The demo keeps the catalog too: the walkthrough's
 * install slide installs cert-manager straight from that grid.
 *
 * Must be called in an injection context.
 */
export default function injectBrowsePluginsUrl(): string {
  if (inject(PRESENTATION_ENABLED)) return '';
  return (inject(ConfigService).getConfig().marketplaceUrl ?? '').replace(/\/+$/, '');
}
