import { positive } from '../utils/defaults';
import {
  CPU_SECTION,
  MEMORY_SECTION,
  type ResourceSectionCopy,
} from '../resource-defaults-section/resource-defaults-section.component';

/**
 * The four per-container defaults as the console holds them: undefined is the
 * absent proto field, which means "no value of its own".
 */
export interface ContainerDefaultValues {
  memoryRequestMi: number | undefined;
  memoryLimitMi: number | undefined;
  cpuRequestM: number | undefined;
  cpuLimitM: number | undefined;
}

export const NO_DEFAULTS: ContainerDefaultValues = {
  memoryRequestMi: undefined,
  memoryLimitMi: undefined,
  cpuRequestM: undefined,
  cpuLimitM: undefined,
};

/** What the API returned, with a proto zero read as "not set". */
export function valuesFrom(
  defaults:
    | {
        memoryRequestMi?: number;
        memoryLimitMi?: number;
        cpuRequestM?: number;
        cpuLimitM?: number;
      }
    | undefined,
): ContainerDefaultValues {
  return {
    memoryRequestMi: positive(defaults?.memoryRequestMi),
    memoryLimitMi: positive(defaults?.memoryLimitMi),
    cpuRequestM: positive(defaults?.cpuRequestM),
    cpuLimitM: positive(defaults?.cpuLimitM),
  };
}

/** One row of the read-only summary: a label and where its value comes from. */
export interface DefaultsSummaryRow {
  label: string;
  /** The value as a number with its unit, or null when nothing applies. */
  value: string | null;
  /** Says the value is the cluster's, not this project's. */
  inherited: boolean;
}

/** One summary section: a heading and its request/limit rows. */
export interface DefaultsSummarySection {
  copy: ResourceSectionCopy;
  rows: DefaultsSummaryRow[];
}

function row(
  label: string,
  own: number | undefined,
  inheritable: number | undefined,
  unit: string,
): DefaultsSummaryRow {
  if (own !== undefined) return { label, value: `${own} ${unit}`, inherited: false };
  if (inheritable !== undefined) return { label, value: `${inheritable} ${unit}`, inherited: true };
  return { label, value: null, inherited: false };
}

function section(
  copy: ResourceSectionCopy,
  request: number | undefined,
  limit: number | undefined,
  cluster: ContainerDefaultValues,
  resource: 'memory' | 'cpu',
): DefaultsSummarySection {
  const clusterRequest = resource === 'memory' ? cluster.memoryRequestMi : cluster.cpuRequestM;
  const clusterLimit = resource === 'memory' ? cluster.memoryLimitMi : cluster.cpuLimitM;
  return {
    copy,
    rows: [
      row(copy.requestName, request, clusterRequest, copy.unit),
      row(copy.limitName, limit, clusterLimit, copy.unit),
    ],
  };
}

/**
 * The summary for a cluster: its own values, or nothing at all. A cluster has
 * nothing above it, so a field it does not set applies no default.
 */
export function clusterSummary(values: ContainerDefaultValues): DefaultsSummarySection[] {
  return [
    section(MEMORY_SECTION, values.memoryRequestMi, values.memoryLimitMi, NO_DEFAULTS, 'memory'),
    section(CPU_SECTION, values.cpuRequestM, values.cpuLimitM, NO_DEFAULTS, 'cpu'),
  ];
}

/**
 * The summary for a project: its own value where it has one, the cluster's
 * where it does not, and "Not set" where neither has one.
 */
export function projectSummary(
  values: ContainerDefaultValues,
  cluster: ContainerDefaultValues,
): DefaultsSummarySection[] {
  return [
    section(MEMORY_SECTION, values.memoryRequestMi, values.memoryLimitMi, cluster, 'memory'),
    section(CPU_SECTION, values.cpuRequestM, values.cpuLimitM, cluster, 'cpu'),
  ];
}
