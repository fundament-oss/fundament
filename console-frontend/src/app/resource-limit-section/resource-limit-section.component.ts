import {
  Component,
  ChangeDetectionStrategy,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
  model,
  type WritableSignal,
} from '@angular/core';
import { toInt } from '../utils/limits';

/**
 * What a pair is set to. Defaults and custom both hold values and both let you
 * edit them; the difference is only whether those values are still the
 * platform's, which is worth saying out loud on a page about limits.
 */
export type ResourceMode = 'unlimited' | 'defaults' | 'custom';

/** A pair of platform values to fall back on. */
export interface ResourceSeed {
  request: number | undefined;
  limit: number | undefined;
}

/**
 * Everything that differs between the memory and the CPU section: only words.
 * The unit is its own field, because it belongs to the label of an empty input
 * but to the value of a number that is already there.
 */
export interface ResourceSectionCopy {
  title: string;
  description: string;
  unit: string;
  requestId: string;
  requestName: string;
  requestAccessibleLabel: string;
  limitId: string;
  limitName: string;
  limitAccessibleLabel: string;
  /** Name for the toggle group, so the two radios do not share a group. */
  name: string;
}

export const MEMORY_SECTION: ResourceSectionCopy = {
  title: 'Memory per container',
  description:
    'Unlimited means containers without their own memory settings run without a request or limit. Memory is in mebibytes (MiB).',
  unit: 'MiB',
  requestId: 'defaultMemoryRequest',
  requestName: 'Default memory request',
  requestAccessibleLabel: 'Default memory request in mebibytes',
  limitId: 'defaultMemoryLimit',
  limitName: 'Default memory limit',
  limitAccessibleLabel: 'Default memory limit in mebibytes',
  name: 'memoryMode',
};

export const CPU_SECTION: ResourceSectionCopy = {
  title: 'CPU per container',
  description:
    'Unlimited means containers without their own CPU settings run without a request or limit. CPU is in millicores (m), where 1000 m equals 1 vCPU.',
  unit: 'millicores',
  requestId: 'defaultCpuRequest',
  requestName: 'Default CPU request',
  requestAccessibleLabel: 'Default CPU request in millicores',
  limitId: 'defaultCpuLimit',
  limitName: 'Default CPU limit',
  limitAccessibleLabel: 'Default CPU limit in millicores',
  name: 'cpuMode',
};

/** Which of the three a stored pair amounts to, for the initial selection. */
export function modeFor(
  request: number | undefined,
  limit: number | undefined,
  seed: ResourceSeed,
): ResourceMode {
  if (request === undefined && limit === undefined) return 'unlimited';
  if (request === seed.request && limit === seed.limit) return 'defaults';
  return 'custom';
}

/**
 * One limited resource: a request and a limit that switch as a pair, because a
 * LimitRange with neither is no constraint at all.
 *
 * The owning page keeps the values, so it can save and reload them; this
 * component owns what picking a mode does to them. It is a section rather than
 * the whole form, so a page can put it straight into an nldd-form and let the
 * form space it like any other child.
 */
@Component({
  selector: 'app-resource-limit-section',
  templateUrl: './resource-limit-section.component.html',
  // An Angular host is an unknown element, so it is inline by default and
  // nldd-form's rhythm (a margin on the bottom of each child) would silently do
  // nothing to it. A block takes part like any other form child.
  styles: ':host { display: block; }',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class ResourceLimitSectionComponent {
  copy = input.required<ResourceSectionCopy>();

  /** Seeds a pair the user switches on, so a mode starts somewhere sensible. */
  seed = input.required<ResourceSeed>();

  mode = model.required<ResourceMode>();

  request = model<number | undefined>(undefined);

  limit = model<number | undefined>(undefined);

  protected readonly toInt = toInt;

  /** An empty field cannot carry its unit, so the label does. */
  protected requestLabel = computed(() => `${this.copy().requestName} (${this.copy().unit})`);

  protected limitLabel = computed(() => `${this.copy().limitName} (${this.copy().unit})`);

  /**
   * Unlimited stores undefined, which is how the API encodes "no default set";
   * defaults writes the platform's values; custom keeps what is there and only
   * fills the halves that are empty, so a saved value is never overwritten.
   */
  protected select(mode: ResourceMode): void {
    const seed = this.seed();
    this.mode.set(mode);
    if (mode === 'unlimited') {
      this.request.set(undefined);
      this.limit.set(undefined);
      return;
    }
    if (mode === 'defaults') {
      this.request.set(seed.request);
      this.limit.set(seed.limit);
      return;
    }
    if (this.request() === undefined) this.request.set(seed.request);
    if (this.limit() === undefined) this.limit.set(seed.limit);
  }

  /**
   * The selection describes the values, so it follows them both ways: editing a
   * platform default makes it yours, and typing the platform's number back makes
   * it theirs again. Unlimited stays a deliberate click, or clearing a field
   * while typing would fold the section away.
   */
  protected edit(field: WritableSignal<number | undefined>, value: number | undefined): void {
    field.set(value);
    const seed = this.seed();
    const isDefault = this.request() === seed.request && this.limit() === seed.limit;
    this.mode.set(isDefault ? 'defaults' : 'custom');
  }
}
