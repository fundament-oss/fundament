import { Component, Input, forwardRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ControlValueAccessor, NG_VALUE_ACCESSOR, FormsModule } from '@angular/forms';

export interface SelectOption {
  label: string;
  value: string | number;
  disabled?: boolean;
}

@Component({
  selector: 'app-form-select',
  standalone: true,
  imports: [CommonModule, FormsModule],
  providers: [
    {
      provide: NG_VALUE_ACCESSOR,
      useExisting: forwardRef(() => FormSelectComponent),
      multi: true,
    },
  ],
  template: `
    <div>
      @if (label) {
        <label [for]="id" class="mb-2 block text-sm font-medium text-gray-700 dark:text-white">
          {{ label }}
          @if (required) {
            <span class="text-rose-500">*</span>
          }
        </label>
      }
      <select
        [id]="id"
        [disabled]="disabled"
        [required]="required"
        [(ngModel)]="value"
        (blur)="onTouched()"
        class="rounded-md border text-sm ring-indigo-500 focus:border-indigo-500 focus:ring-1 focus:outline-none dark:bg-gray-900 dark:text-white dark:ring-offset-gray-950"
        [class.w-full]="fullWidth"
        [class.border-gray-300]="!error"
        [class.dark:border-gray-800]="!error"
        [class.border-rose-500]="error"
        [class.ring-rose-500]="error"
        [class.focus:border-rose-500]="error"
      >
        @if (placeholder) {
          <option value="">{{ placeholder }}</option>
        }
        @for (option of options; track option.value) {
          <option [value]="option.value" [disabled]="option.disabled">
            {{ option.label }}
          </option>
        }
      </select>
      @if (helpText && !error) {
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ helpText }}</p>
      }
      @if (error) {
        <p class="mt-2 text-sm text-rose-600 dark:text-rose-400">{{ error }}</p>
      }
    </div>
  `,
})
export class FormSelectComponent implements ControlValueAccessor {
  @Input() id = '';
  @Input() label = '';
  @Input() options: SelectOption[] = [];
  @Input() placeholder = '';
  @Input() helpText = '';
  @Input() error = '';
  @Input() required = false;
  @Input() disabled = false;
  @Input() fullWidth = true;

  private _value: string | number = '';

  get value(): string | number {
    return this._value;
  }

  set value(val: string | number) {
    this._value = val;
    this.onChange(val);
  }

  // eslint-disable-next-line @typescript-eslint/no-empty-function
  onChange: (value: string | number) => void = () => {};
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  onTouched: () => void = () => {};

  writeValue(value: string | number): void {
    this._value = value;
  }

  registerOnChange(fn: (value: string | number) => void): void {
    this.onChange = fn;
  }

  registerOnTouched(fn: () => void): void {
    this.onTouched = fn;
  }

  setDisabledState(isDisabled: boolean): void {
    this.disabled = isDisabled;
  }
}
