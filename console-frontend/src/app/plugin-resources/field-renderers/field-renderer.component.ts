import { Component, ChangeDetectionStrategy, input, CUSTOM_ELEMENTS_SCHEMA } from '@angular/core';
import type { CrdPropertySchema } from '../types';
import { toDateValue, toSimpleValue, fieldNameToLabel } from '../crd-schema.utils';

function toArray(val: unknown): unknown[] {
  return Array.isArray(val) ? val : [];
}

function toObjectEntries(val: unknown): [string, unknown][] {
  if (!val || typeof val !== 'object' || Array.isArray(val)) return [];
  return Object.entries(val as Record<string, unknown>);
}

@Component({
  selector: 'app-field-renderer',
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @switch (effectiveType()) {
      @case ('date') {
        <span>{{ formatDateValue(value()) }}</span>
      }
      @case ('boolean') {
        <span>{{ value() ? 'Yes' : 'No' }}</span>
      }
      @case ('string-array') {
        @if (asArray(value()).length > 0) {
          <!-- Tekst, geen tags: een domeinnaam is de waarde van het veld en geen
               etiket dat iemand eraan hing. -->
          <span>{{ asArray(value()).join(', ') }}</span>
        } @else {
          <span class="text-gray-500 dark:text-gray-400">&mdash;</span>
        }
      }
      @case ('object-array') {
        @if (asArray(value()).length > 0) {
          <div class="space-y-2">
            @for (item of asArray(value()); track $index) {
              <div class="rounded border border-gray-200 p-2 dark:border-gray-700">
                @for (entry of objectEntries(item); track entry[0]) {
                  <div>
                    <span class="font-medium text-gray-600 dark:text-gray-400"
                      >{{ formatLabel(entry[0]) }}:</span
                    >
                    {{ formatSimpleValue(entry[1]) }}
                  </div>
                }
              </div>
            }
          </div>
        } @else {
          <span class="text-gray-500 dark:text-gray-400">&mdash;</span>
        }
      }
      @case ('object') {
        @if (value() && objectEntries(value()).length > 0) {
          <dl class="space-y-1">
            @for (entry of objectEntries(value()); track entry[0]) {
              <div>
                <dt class="inline font-medium text-gray-600 dark:text-gray-400">
                  {{ formatLabel(entry[0]) }}:
                </dt>
                <dd class="ml-1 inline">{{ formatSimpleValue(entry[1]) }}</dd>
              </div>
            }
          </dl>
        } @else {
          <span class="text-gray-500 dark:text-gray-400">&mdash;</span>
        }
      }
      @default {
        @if (value() !== null && value() !== undefined && value() !== '') {
          <span>{{ value() }}</span>
        } @else {
          <span class="text-gray-500 dark:text-gray-400">&mdash;</span>
        }
      }
    }
  `,
})
export default class FieldRendererComponent {
  schema = input.required<CrdPropertySchema>();

  value = input.required<unknown>();

  effectiveType(): string {
    const s = this.schema();
    if (s.format === 'date-time') return 'date';
    if (s.type === 'boolean') return 'boolean';
    if (s.type === 'array' && s.items?.type === 'string') return 'string-array';
    if (s.type === 'array' && s.items?.type === 'object') return 'object-array';
    if (s.type === 'object') return 'object';
    return 'text';
  }

  formatDateValue = toDateValue;

  formatLabel = fieldNameToLabel;

  formatSimpleValue = toSimpleValue;

  asArray = toArray;

  objectEntries = toObjectEntries;
}
