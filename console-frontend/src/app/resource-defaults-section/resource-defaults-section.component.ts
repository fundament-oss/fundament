import {
  Component,
  ChangeDetectionStrategy,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
  model,
  type WritableSignal,
} from '@angular/core';
import { INT32_MAX, toInt } from '../utils/defaults';

import '@nldd/design-system/cell';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/number-field';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/toggle-button';
import '@nldd/design-system/toggle-button-group';
/**
 * What a pair is set to. The states differ per context, so the mode does too:
 * a cluster can set nothing, take the platform's suggestion or pick its own; a
 * project either inherits the cluster's values or sets its own, lower ones.
 * "Defaults" would be ambiguous now that the values themselves are defaults.
 */
export type ResourceMode = 'none' | 'suggested' | 'inherit' | 'custom';

/** Whose defaults a section edits, which decides the modes it offers. */
export type DefaultsScope = 'cluster' | 'project';

/** A pair to fall back on: the platform's suggestion, or the cluster's values. */
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
  clusterDescription: string;
  projectDescription: string;
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
  clusterDescription:
    'Not set means containers without their own memory settings run without a request or limit. Memory is in mebibytes (MiB).',
  projectDescription:
    "Inherited means this project uses the cluster's memory defaults. Memory is in mebibytes (MiB).",
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
  clusterDescription:
    'Not set means containers without their own CPU settings run without a request or limit. CPU is in millicores (m), where 1000 m equals 1 vCPU.',
  projectDescription:
    "Inherited means this project uses the cluster's CPU defaults. CPU is in millicores (m), where 1000 m equals 1 vCPU.",
  unit: 'millicores',
  requestId: 'defaultCpuRequest',
  requestName: 'Default CPU request',
  requestAccessibleLabel: 'Default CPU request in millicores',
  limitId: 'defaultCpuLimit',
  limitName: 'Default CPU limit',
  limitAccessibleLabel: 'Default CPU limit in millicores',
  name: 'cpuMode',
};

/** The smaller of two optional numbers, or whichever one is there. */
function lowest(a: number | undefined, b: number | undefined): number | undefined {
  if (a === undefined) return b;
  if (b === undefined) return a;
  return Math.min(a, b);
}

function ceilingHint(ceiling: number | undefined, unit: string): string | null {
  return ceiling === undefined ? null : `At most ${ceiling} ${unit}, the cluster's default`;
}

/** The mode that stores nothing, which differs in what it then means. */
export const offMode = (scope: DefaultsScope): ResourceMode =>
  scope === 'cluster' ? 'none' : 'inherit';

/** Which mode a stored pair amounts to, for the initial selection. */
export function modeFor(
  scope: DefaultsScope,
  request: number | undefined,
  limit: number | undefined,
  seed: ResourceSeed,
): ResourceMode {
  if (request === undefined && limit === undefined) return offMode(scope);
  if (scope === 'cluster' && request === seed.request && limit === seed.limit) return 'suggested';
  return 'custom';
}

/**
 * One resource's defaults: a request and a limit that switch as a pair, because
 * a LimitRange with neither is no default at all.
 *
 * The owning page keeps the values, so it can save and reload them; this
 * component owns what picking a mode does to them. It is a section rather than
 * the whole form, so a page can put it straight into an nldd-form and let the
 * form space it like any other child.
 */
@Component({
  selector: 'app-resource-defaults-section',
  templateUrl: './resource-defaults-section.component.html',
  // An Angular host is an unknown element, so it is inline by default and
  // nldd-form's rhythm (a margin on the bottom of each child) would silently do
  // nothing to it. A block takes part like any other form child.
  styles: ':host { display: block; }',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class ResourceDefaultsSectionComponent {
  copy = input.required<ResourceSectionCopy>();

  scope = input.required<DefaultsScope>();

  /** Seeds a pair the user switches on, so a mode starts somewhere sensible:
   *  the platform's suggestion on a cluster, the cluster's values on a project. */
  seed = input.required<ResourceSeed>();

  /** The highest a project may go per field, which is the cluster's value where
   *  it has one. Absent on a cluster, where nothing is above it. */
  ceiling = input<ResourceSeed>({ request: undefined, limit: undefined });

  /** Off while the owning form saves: what is sent is a snapshot, and an edit
   *  made meanwhile would be lost when the sheet closes on success. */
  disabled = input(false);

  mode = model.required<ResourceMode>();

  request = model<number | undefined>(undefined);

  limit = model<number | undefined>(undefined);

  protected readonly toInt = toInt;

  protected description = computed(() =>
    this.scope() === 'cluster' ? this.copy().clusterDescription : this.copy().projectDescription,
  );

  protected offText = computed(() => (this.scope() === 'cluster' ? 'Not set' : 'Inherit'));

  protected isOff = computed(() => this.mode() === offMode(this.scope()));

  /** An empty field cannot carry its unit, so the label does. */
  protected requestLabel = computed(() => `${this.copy().requestName} (${this.copy().unit})`);

  protected limitLabel = computed(() => `${this.copy().limitName} (${this.copy().unit})`);

  /** What a field may not go above: the cluster's value, and for a request also
   *  the limit in this same pair, which on a project is the cluster's limit when
   *  the project sets none. Without any of those, the API's int32. */
  protected requestMax = computed(
    () => lowest(this.limit() ?? this.ceiling().limit, this.ceiling().request) ?? INT32_MAX,
  );

  protected limitMax = computed(() => this.ceiling().limit ?? INT32_MAX);

  /** Says the ceiling out loud, so the number field is not the only place it
   *  shows up — a disabled spin button explains nothing. */
  protected requestHint = computed(() => ceilingHint(this.ceiling().request, this.copy().unit));

  protected limitHint = computed(() => ceilingHint(this.ceiling().limit, this.copy().unit));

  /**
   * The off mode stores undefined, which is how the API encodes "no value of
   * its own"; suggested writes the platform's values; custom keeps what is
   * there and only fills the halves that are empty, so a saved value is never
   * overwritten.
   */
  protected select(mode: ResourceMode): void {
    const seed = this.seed();
    this.mode.set(mode);
    if (mode === offMode(this.scope())) {
      this.request.set(undefined);
      this.limit.set(undefined);
      return;
    }
    if (mode === 'suggested') {
      this.request.set(seed.request);
      this.limit.set(seed.limit);
      return;
    }
    if (this.request() === undefined) this.request.set(seed.request);
    if (this.limit() === undefined) this.limit.set(seed.limit);
  }

  /**
   * The selection describes the values, so it follows them both ways: editing a
   * suggested value makes it the cluster's own, and typing the suggestion back
   * makes it the platform's again. The off mode stays a deliberate click, or
   * clearing a field while typing would fold the section away.
   */
  protected edit(field: WritableSignal<number | undefined>, value: number | undefined): void {
    field.set(value);
    this.mode.set(
      modeFor(this.scope(), this.request(), this.limit(), this.seed()) === 'suggested'
        ? 'suggested'
        : 'custom',
    );
  }
}
