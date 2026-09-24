import {
  Component,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  OnInit,
  computed,
  input,
  output,
  signal,
} from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { ConfigSchemaEntry, ConfigType } from '../../generated/catalog/v1/catalog_pb';

import '@nldd/design-system/button';
import '@nldd/design-system/checkbox';
import '@nldd/design-system/form';
import '@nldd/design-system/form-actions';
import '@nldd/design-system/form-field';
import '@nldd/design-system/spacer';
import '@nldd/design-system/text-field';
import '@nldd/design-system/toggle-button';
import '@nldd/design-system/toggle-button-group';

const INT_PATTERN = /^-?\d+$/;

/**
 * A schema key is a manifest identifier (SCREAMING_SNAKE_CASE, e.g.
 * "MON_COUNT"), never sentence case, so it cannot be shown as-is without
 * breaking the content rule. Humanized for display only — "MON_COUNT" →
 * "Mon count" — same spirit as fieldNameToLabel in plugin-resources/
 * crd-schema.utils.ts, but for underscore-separated names rather than
 * camelCase ones. entry.name itself stays untouched everywhere else: it is
 * the FormControl name and the key the server contract expects.
 */
function humanizeConfigName(name: string): string {
  const words = name.toLowerCase().replace(/_/g, ' ');
  return words.charAt(0).toUpperCase() + words.slice(1);
}

/**
 * The label shown for an entry: its curated displayName when the manifest
 * author set one, else the humanized key. Used for every visible AND
 * accessible label (form-field label, required-field header, enum
 * accessible-label, validation error messages) so they never disagree.
 */
function labelFor(entry: ConfigSchemaEntry): string {
  return entry.displayName || humanizeConfigName(entry.name);
}

/**
 * A control per declared config key (catalog.v1.ConfigSchemaEntry), pre-filled
 * with its default. Submitting emits only what differs from the default, plus
 * every required key: a cleared optional field omits its key entirely, since
 * absence means "use the default" and an empty string would be rejected
 * server-side.
 */
@Component({
  selector: 'app-plugin-config-form',
  imports: [ReactiveFormsModule, NgTemplateOutlet],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <nldd-form
      [formGroup]="form"
      (submit)="onSubmit()"
      (keydown.enter)="onSubmit()"
    >
      @for (entry of basicEntries(); track entry.name) {
        <ng-container
          [ngTemplateOutlet]="field"
          [ngTemplateOutletContext]="{ $implicit: entry }"
        ></ng-container>
      }
      @if (advancedEntries().length > 0) {
        <nldd-button
          type="button"
          variant="secondary"
          size="sm"
          width="full"
          [attr.text]="showAdvanced() ? 'Hide advanced options' : 'Show advanced options'"
          (click)="toggleAdvanced()"
        ></nldd-button>
        <nldd-spacer size="12"></nldd-spacer>
        <!-- Hidden, not removed: a collapsed entry keeps its control so a default
             the user never saw still makes it into the submitted diff logic. -->
        <div [hidden]="!showAdvanced()">
          @for (entry of advancedEntries(); track entry.name) {
            <ng-container
              [ngTemplateOutlet]="field"
              [ngTemplateOutletContext]="{ $implicit: entry }"
            ></ng-container>
          }
        </div>
      }
      <nldd-form-actions>
        <nldd-button
          type="button"
          (click)="onCancel()"
          text="Cancel"
          variant="secondary"
          width="full"
        ></nldd-button>
        <nldd-button
          type="submit"
          [attr.text]="submitLabel()"
          variant="primary"
          width="full"
        ></nldd-button>
      </nldd-form-actions>
    </nldd-form>

    <ng-template
      #field
      let-entry
    >
      <nldd-form-field
        [attr.label]="entry.required ? null : labelFor(entry)"
        [attr.optional]="entry.required ? null : ''"
        optional-label="Optional"
      >
        <!-- nldd-form-field's own label is left off deliberately for required
             entries (see form-field-header in styles.css): the optional badge
             only ever belongs next to an optional field, so a required one gets
             its own plain header instead of the field's built-in label slot. -->
        @if (entry.required) {
          <div class="form-field-header">
            <span class="form-field-header__label">{{ labelFor(entry) }}</span>
          </div>
        }
        @if (entry.type === ConfigType.BOOL) {
          <nldd-checkbox
            [checked]="form.get(entry.name)?.value === true"
            [required]="entry.required"
            [invalid]="isInvalid(entry.name)"
            (change)="onBoolChange(entry.name, $event)"
          ></nldd-checkbox>
        } @else if (entry.type === ConfigType.ENUM) {
          <nldd-toggle-button-group
            type="radio"
            [attr.name]="entry.name"
            [attr.accessible-label]="labelFor(entry)"
            [required]="entry.required"
            [invalid]="isInvalid(entry.name)"
            (change)="onEnumChange(entry.name, $event)"
          >
            @for (value of entry.values; track value) {
              <nldd-toggle-button
                type="radio"
                [value]="value"
                [text]="value"
                [selected]="form.get(entry.name)?.value === value"
              ></nldd-toggle-button>
            }
          </nldd-toggle-button-group>
        } @else {
          <!-- STRING and INT share the same widget; INT is told apart only by
               the pattern validator attached in ngOnInit. -->
          <nldd-text-field
            [value]="form.get(entry.name)?.value"
            [required]="entry.required"
            [invalid]="isInvalid(entry.name)"
            (input)="onTextInput(entry.name, $event)"
          ></nldd-text-field>
        }
        @if (isInvalid(entry.name)) {
          <nldd-form-field-help-text>{{ errorFor(entry) }}</nldd-form-field-help-text>
        } @else if (entry.description) {
          <nldd-form-field-help-text>{{ entry.description }}</nldd-form-field-help-text>
        }
      </nldd-form-field>
    </ng-template>
  `,
})
export default class PluginConfigFormComponent implements OnInit {
  schema = input<ConfigSchemaEntry[]>([]);

  submitLabel = input('Install');

  confirmed = output<Record<string, string>>();

  cancelled = output<void>();

  readonly ConfigType = ConfigType;

  /** The label to show for an entry, for display only — see labelFor.
   *  `entry.name` itself is never shown. */
  readonly labelFor = labelFor;

  // Untyped: the control map is only known at runtime, built from the schema
  // input in ngOnInit.
  form: FormGroup = new FormGroup({});

  showAdvanced = signal(false);

  submitAttempted = signal(false);

  basicEntries = computed(() => this.schema().filter((entry) => !entry.advanced));

  advancedEntries = computed(() => this.schema().filter((entry) => entry.advanced));

  ngOnInit(): void {
    // The modal only renders this component once a schema is known, so
    // building the group once here — rather than reacting to the input signal
    // — is enough; the schema never changes under an already-open form.
    const group: FormGroup = new FormGroup({});
    this.schema().forEach((entry) => {
      const validators = [];
      if (entry.required) validators.push(Validators.required);
      if (entry.type === ConfigType.INT) validators.push(Validators.pattern(INT_PATTERN));
      const initial: string | boolean =
        entry.type === ConfigType.BOOL ? entry.defaultValue === 'true' : entry.defaultValue;
      group.addControl(entry.name, new FormControl(initial, validators));
    });
    this.form = group;
  }

  toggleAdvanced(): void {
    this.showAdvanced.update((value) => !value);
  }

  onTextInput(name: string, event: Event): void {
    const value = (event as CustomEvent<{ value: string }>).detail.value;
    this.form.get(name)?.setValue(value);
  }

  onBoolChange(name: string, event: Event): void {
    const checked = (event as CustomEvent<{ checked: boolean }>).detail.checked;
    this.form.get(name)?.setValue(checked);
  }

  onEnumChange(name: string, event: Event): void {
    const value = (event as CustomEvent<{ value: string }>).detail.value;
    this.form.get(name)?.setValue(value);
  }

  onCancel(): void {
    this.cancelled.emit();
  }

  onSubmit(): void {
    this.submitAttempted.set(true);
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }
    const config: Record<string, string> = {};
    this.schema().forEach((entry) => {
      const raw = this.form.get(entry.name)?.value;
      const value =
        entry.type === ConfigType.BOOL
          ? String(raw === true || raw === 'true')
          : String(raw ?? '').trim();
      if (entry.required) {
        config[entry.name] = value;
      } else if (value !== '' && value !== entry.defaultValue) {
        config[entry.name] = value;
      }
    });
    this.confirmed.emit(config);
  }

  isInvalid(name: string): boolean {
    return this.submitAttempted() && !!this.form.get(name)?.invalid;
  }

  errorFor(entry: ConfigSchemaEntry): string {
    const control = this.form.get(entry.name);
    const label = this.labelFor(entry);
    if (control?.hasError('required')) return `${label} is required.`;
    if (control?.hasError('pattern')) return `${label} must be a whole number.`;
    return '';
  }
}
