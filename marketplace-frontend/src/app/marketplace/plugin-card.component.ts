import { Component, input, ChangeDetectionStrategy, CUSTOM_ELEMENTS_SCHEMA } from '@angular/core';
import { RouterLink } from '@angular/router';
import { PluginIconComponent } from '../icons';
import { type MarketplacePlugin } from './marketplace.service';
import PluginLabelsComponent from './plugin-labels.component';

// Compact plugin tile used across the marketplace home sections and results grid.
@Component({
  selector: 'app-plugin-card',
  imports: [RouterLink, PluginIconComponent, PluginLabelsComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <a [routerLink]="['/plugins', plugin().name]" class="group block h-full">
      <nldd-card
        class="hover:ring-accent-200 dark:hover:ring-accent-800 block h-full transition duration-200 group-hover:-translate-y-1 group-hover:shadow-lg group-hover:ring-1"
      >
        <div class="flex h-full flex-col gap-3 p-5">
          <div class="flex items-start gap-3">
            <app-plugin-icon
              [name]="plugin().icon"
              [label]="plugin().displayName"
              class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-gray-100 p-1.5 dark:bg-gray-800"
            />
            <div class="min-w-0 flex-1">
              <h3
                class="truncate font-semibold text-gray-900 group-hover:underline dark:text-white"
              >
                {{ plugin().displayName }}
              </h3>
              <p class="truncate text-sm text-gray-500 dark:text-gray-400">{{ plugin().vendor }}</p>
            </div>
          </div>
          <p class="line-clamp-2 flex-1 text-sm text-gray-600 dark:text-gray-300">
            {{ plugin().tagline }}
          </p>
          <div class="flex flex-wrap items-center gap-1.5">
            <nldd-tag size="sm" color="neutral" [text]="plugin().category"></nldd-tag>
            <app-plugin-labels [labels]="plugin().labels" />
          </div>
        </div>
      </nldd-card>
    </a>
  `,
})
export default class PluginCardComponent {
  plugin = input.required<MarketplacePlugin>();
}
