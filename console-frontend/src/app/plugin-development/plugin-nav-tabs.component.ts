import { Component, ChangeDetectionStrategy } from '@angular/core';
import { RouterLink, RouterLinkActive } from '@angular/router';

// Shared sub-navigation shown on both the consumer catalog (/plugins) and the
// author hub (/plugins/manage) so the two views feel like one area.
@Component({
  selector: 'app-plugin-nav-tabs',
  imports: [RouterLink, RouterLinkActive],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <nav class="border-dividers mb-6 flex gap-6 border-b" aria-label="Plugin sections">
      <a
        routerLink="/plugins"
        routerLinkActive
        #catalogActive="routerLinkActive"
        [routerLinkActiveOptions]="{ exact: true }"
        [class]="
          catalogActive.isActive
            ? 'border-accent-650 text-content'
            : 'text-content-secondary border-transparent'
        "
        class="-mb-px border-b-2 px-1 py-2.5 text-lg font-semibold"
        >Catalog</a
      >
      <a
        routerLink="/plugins/manage"
        routerLinkActive
        #mineActive="routerLinkActive"
        [class]="
          mineActive.isActive
            ? 'border-accent-650 text-content'
            : 'text-content-secondary border-transparent'
        "
        class="-mb-px border-b-2 px-1 py-2.5 text-lg font-semibold"
        >My plugins</a
      >
    </nav>
  `,
})
export default class PluginNavTabsComponent {}
