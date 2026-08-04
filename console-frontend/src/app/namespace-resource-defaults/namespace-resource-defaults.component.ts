import {
  Component,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
  model,
  type WritableSignal,
} from '@angular/core';
import { toInt } from '../utils/limits';

/** Platform defaults for a namespace LimitRange, as returned by the API. */
export interface NamespaceDefaults {
  defaultMemoryRequestMi: number | undefined;
  defaultMemoryLimitMi: number | undefined;
  defaultCpuRequestM: number | undefined;
  defaultCpuLimitM: number | undefined;
}

/**
 * Request and limit switch as one pair: a LimitRange with neither is no
 * constraint at all. Off stores undefined, which is how the API encodes "no
 * default set"; on fills only the halves the user left empty, so a saved value is
 * never overwritten by a platform default.
 */
function togglePair(
  limited: boolean,
  toggle: WritableSignal<boolean>,
  request: WritableSignal<number | undefined>,
  limit: WritableSignal<number | undefined>,
  seed: { request: number | undefined; limit: number | undefined },
): void {
  toggle.set(limited);
  if (!limited) {
    request.set(undefined);
    limit.set(undefined);
    return;
  }
  if (request() === undefined) request.set(seed.request);
  if (limit() === undefined) limit.set(seed.limit);
}

/**
 * The memory and CPU halves of a namespace LimitRange form, shared by the
 * organization and project limits pages: both edit the same four values with the
 * same copy, and only the surrounding card and the save call differ.
 *
 * The owning page keeps the values, so it can save and reset them; this
 * component owns what switching a pair on or off does to them.
 */
@Component({
  selector: 'app-namespace-resource-defaults',
  templateUrl: './namespace-resource-defaults.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class NamespaceResourceDefaultsComponent {
  /** Seeds a pair the user switches on, so "Limited" starts somewhere sensible. */
  defaults = input.required<NamespaceDefaults>();

  memoryLimited = model.required<boolean>();

  memoryRequestMi = model<number | undefined>(undefined);

  memoryLimitMi = model<number | undefined>(undefined);

  cpuLimited = model.required<boolean>();

  cpuRequestM = model<number | undefined>(undefined);

  cpuLimitM = model<number | undefined>(undefined);

  protected readonly toInt = toInt;

  protected toggleMemory(limited: boolean): void {
    const defaults = this.defaults();
    togglePair(limited, this.memoryLimited, this.memoryRequestMi, this.memoryLimitMi, {
      request: defaults.defaultMemoryRequestMi,
      limit: defaults.defaultMemoryLimitMi,
    });
  }

  protected toggleCpu(limited: boolean): void {
    const defaults = this.defaults();
    togglePair(limited, this.cpuLimited, this.cpuRequestM, this.cpuLimitM, {
      request: defaults.defaultCpuRequestM,
      limit: defaults.defaultCpuLimitM,
    });
  }
}
