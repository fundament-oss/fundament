import { Component, Input, forwardRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ControlValueAccessor, NG_VALUE_ACCESSOR, FormsModule } from '@angular/forms';

@Component({
  selector: 'app-form-input',
  standalone: true,
  imports: [CommonModule, FormsModule],
  providers: [
    {
      provide: NG_VALUE_ACCESSOR,
      useExisting: forwardRef(() => FormInputComponent),
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
      <input
        [id]="id"
        [type]="type"
        [placeholder]="placeholder"
        [disabled]="disabled"
        [required]="required"
        [(ngModel)]="value"
        (blur)="onTouched()"
        class="w-full rounded-md border px-3 py-2 text-sm placeholder-gray-400 ring-indigo-500 focus:border-indigo-500 focus:ring-1 focus:outline-none dark:bg-gray-900 dark:text-white dark:placeholder-gray-600 dark:ring-offset-gray-950"
        [class.border-gray-300]="!error"
        [class.dark:border-gray-800]="!error"
        [class.border-rose-500]="error"
        [class.ring-rose-500]="error"
        [class.focus:border-rose-500]="error"
      />
      @if (helpText && !error) {
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ helpText }}</p>
      }
      @if (error) {
        <p class="mt-2 text-sm text-rose-600 dark:text-rose-400">{{ error }}</p>
      }
    </div>
  `,
})
export class FormInputComponent implements ControlValueAccessor {
  @Input() id = '';
  @Input() label = '';
  @Input() type: 'text' | 'email' | 'password' | 'number' | 'date' = 'text';
  @Input() placeholder = '';
  @Input() helpText = '';
  @Input() error = '';
  @Input() required = false;
  @Input() disabled = false;

  private _value = '';

  get value(): string {
    return this._value;
  }

  set value(val: string) {
    this._value = val;
    this.onChange(val);
  }

  // eslint-disable-next-line @typescript-eslint/no-empty-function
  onChange: (value: string) => void = () => {};
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  onTouched: () => void = () => {};

  writeValue(value: string): void {
    this._value = value;
  }

  registerOnChange(fn: (value: string) => void): void {
    this.onChange = fn;
  }

  registerOnTouched(fn: () => void): void {
    this.onTouched = fn;
  }

  setDisabledState(isDisabled: boolean): void {
    this.disabled = isDisabled;
  }
}
