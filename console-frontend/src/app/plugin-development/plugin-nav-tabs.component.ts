import { Component, ChangeDetectionStrategy } from '@angular/core';
import { RouterLink, RouterLinkActive } from '@angular/router';

// Shared sub-navigation shown on both the consumer catalog (/plugins) and the
// author hub (/plugins/manage) so the two views feel like one area.
@Component({
  selector: 'app-plugin-nav-tabs',
  imports: [RouterLink, RouterLinkActive],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <nav class="mb-6 flex gap-1 border-b border-gray-200 dark:border-gray-800">
      <a
        routerLink="/plugins"
        routerLinkActive="border-accent-500 text-accent-700 dark:text-accent-300"
        [routerLinkActiveOptions]="{ exact: true }"
        class="-mb-px border-b-2 border-transparent px-4 py-2 font-medium text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
      >
        Catalog
      </a>
      <a
        routerLink="/plugins/manage"
        routerLinkActive="border-accent-500 text-accent-700 dark:text-accent-300"
        class="-mb-px border-b-2 border-transparent px-4 py-2 font-medium text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
      >
        My plugins
      </a>
    </nav>
  `,
})
export default class PluginNavTabsComponent {}
