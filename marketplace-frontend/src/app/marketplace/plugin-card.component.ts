import { Component, input, ChangeDetectionStrategy, CUSTOM_ELEMENTS_SCHEMA } from '@angular/core';
import { RouterLink } from '@angular/router';
import { PluginIconComponent } from '../icons';
import { type MarketplacePlugin } from './marketplace.service';

// Compact plugin tile used across the marketplace home sections and results grid.
@Component({
  selector: 'app-plugin-card',
  imports: [RouterLink, PluginIconComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <a [routerLink]="['/plugins', plugin().name]" class="group block h-full">
      <nldd-card class="block h-full">
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
            @if (plugin().official) {
              <nldd-tag size="sm" color="success" icon="check-mark" text="Official"></nldd-tag>
            }
          </div>
          <p class="line-clamp-2 flex-1 text-sm text-gray-600 dark:text-gray-300">
            {{ plugin().tagline }}
          </p>
          <div>
            <nldd-tag size="sm" color="neutral" [text]="plugin().category"></nldd-tag>
          </div>
        </div>
      </nldd-card>
    </a>
  `,
})
export default class PluginCardComponent {
  plugin = input.required<MarketplacePlugin>();
}
