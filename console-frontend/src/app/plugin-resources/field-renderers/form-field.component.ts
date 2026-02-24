import { Component, ChangeDetectionStrategy, input, output } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerPlus, tablerX } from '@ng-icons/tabler-icons';
import type { CrdPropertySchema } from '../types';
import { fieldNameToLabel } from '../crd-schema.utils';

function toStringArray(val: unknown): string[] {
  return Array.isArray(val) ? (val as string[]) : [];
}

@Component({
  selector: 'app-form-field',
  standalone: true,
  imports: [FormsModule, NgIcon],
  viewProviders: [provideIcons({ tablerPlus, tablerX })],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="space-y-1">
      <label
        [attr.for]="'field-' + fieldName()"
        class="block text-sm font-medium text-gray-700 dark:text-gray-300"
      >
        {{ label() }}
        @if (required()) {
          <span class="text-rose-500">*</span>
        }
      </label>

      @if (schema().description) {
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ schema().description }}</p>
      }

      @switch (effectiveType()) {
        @case ('enum') {
          <select
            [id]="'field-' + fieldName()"
            [ngModel]="value()"
            (ngModelChange)="valueChange.emit($event)"
            class="w-full"
          >
            <option value="">Select...</option>
            @for (opt of schema().enum ?? []; track opt) {
              <option [value]="opt">{{ opt }}</option>
            }
          </select>
        }
        @case ('boolean') {
          <label class="flex cursor-pointer items-center gap-2">
            <input
              type="checkbox"
              [id]="'field-' + fieldName()"
              class="peer sr-only"
              [ngModel]="value()"
              (ngModelChange)="valueChange.emit($event)"
            />
            <span class="switch"></span>
            <span class="text-sm text-gray-600 dark:text-gray-400">{{ label() }}</span>
          </label>
        }
        @case ('integer') {
          <input
            type="number"
            [id]="'field-' + fieldName()"
            [ngModel]="value()"
            (ngModelChange)="valueChange.emit($event)"
            [placeholder]="label()"
          />
        }
        @case ('string-array') {
          <div class="space-y-2">
            @if (asArray(value()).length > 0) {
              <div class="flex flex-wrap gap-1.5">
                @for (item of asArray(value()); track $index) {
                  <span class="badge badge-blue inline-flex items-center gap-1">
                    {{ item }}
                    <button
                      type="button"
                      (click)="removeArrayItem($index)"
                      class="cursor-pointer hover:text-blue-200"
                      aria-label="Remove"
                    >
                      <ng-icon name="tablerX" size="0.75rem" />
                    </button>
                  </span>
                }
              </div>
            }
            <div class="flex gap-2">
              <input
                type="text"
                [id]="'field-' + fieldName()"
                [(ngModel)]="newArrayItem"
                placeholder="Add item..."
                class="flex-1"
                (keydown.enter)="addArrayItem(); $event.preventDefault()"
              />
              <button
                type="button"
                (click)="addArrayItem()"
                class="btn-secondary inline-flex items-center"
              >
                <ng-icon name="tablerPlus" size="1rem" class="mr-1" />
                Add
              </button>
            </div>
          </div>
        }
        @case ('object') {
          <div
            class="rounded-md border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-900"
          >
            @for (entry of objectFields(); track entry[0]) {
              <div class="mb-3 last:mb-0">
                <app-form-field
                  [schema]="entry[1]"
                  [fieldName]="entry[0]"
                  [value]="getNestedValue(entry[0])"
                  [required]="isNestedRequired(entry[0])"
                  (valueChange)="updateNestedValue(entry[0], $event)"
                />
              </div>
            }
          </div>
        }
        @case ('empty-object') {
          <label class="flex cursor-pointer items-center gap-2">
            <input
              type="checkbox"
              [id]="'field-' + fieldName()"
              class="peer sr-only"
              [ngModel]="value() !== null && value() !== undefined"
              (ngModelChange)="valueChange.emit($event ? {} : null)"
            />
            <span class="switch"></span>
            <span class="text-sm text-gray-600 dark:text-gray-400">Enabled</span>
          </label>
        }
        @default {
          <input
            type="text"
            [id]="'field-' + fieldName()"
            [ngModel]="value()"
            (ngModelChange)="valueChange.emit($event)"
            [placeholder]="label()"
          />
        }
      }
    </div>
  `,
})
export default class FormFieldComponent {
  schema = input.required<CrdPropertySchema>();

  fieldName = input.required<string>();

  value = input.required<unknown>();

  required = input<boolean>(false);

  valueChange = output<unknown>();

  newArrayItem = '';

  effectiveType(): string {
    const s = this.schema();
    if (s.enum && s.enum.length > 0) return 'enum';
    if (s.type === 'boolean') return 'boolean';
    if (s.type === 'integer' || s.type === 'number') return 'integer';
    if (s.type === 'array' && s.items?.type === 'string') return 'string-array';
    if (s.type === 'object' && s.properties && Object.keys(s.properties).length > 0)
      return 'object';
    if (s.type === 'object') return 'empty-object';
    return 'text';
  }

  label(): string {
    return fieldNameToLabel(this.fieldName());
  }

  asArray = toStringArray;

  addArrayItem(): void {
    if (!this.newArrayItem.trim()) return;
    const current = this.asArray(this.value());
    this.valueChange.emit([...current, this.newArrayItem.trim()]);
    this.newArrayItem = '';
  }

  removeArrayItem(index: number): void {
    const current = this.asArray(this.value());
    this.valueChange.emit(current.filter((_, i) => i !== index));
  }

  objectFields(): [string, CrdPropertySchema][] {
    const props = this.schema().properties;
    if (!props) return [];
    return Object.entries(props);
  }

  getNestedValue(key: string): unknown {
    const obj = this.value() as Record<string, unknown> | null;
    return obj?.[key] ?? null;
  }

  isNestedRequired(key: string): boolean {
    return this.schema().required?.includes(key) ?? false;
  }

  updateNestedValue(key: string, newValue: unknown): void {
    const current = (this.value() as Record<string, unknown>) ?? {};
    this.valueChange.emit({ ...current, [key]: newValue });
  }
}
