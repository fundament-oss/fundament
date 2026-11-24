import toIsoDate from '../../connect/timestamp';
import {
  Change,
  ImageAnalysisStatus,
  type ChangeSummary as ProtoChangeSummary,
  type ImageAnalysis as ProtoImageAnalysis,
} from '../../generated/admin/v1/common_pb';
import { type GetImageAnalysisResponse } from '../../generated/admin/v1/review_pb';

// What a version's container image contains, as admin.v1.GetImageAnalysis
// serves it (FUN-24): its packages and files, diffed against a base version.

// 'none': the version has no analysis yet (the worker's sweep queues it).
export type AnalysisStatus = 'none' | 'pending' | 'running' | 'retrying' | 'completed' | 'failed';

// 'none': not compared, because there is no base or one side is not done.
export type ChangeKind = 'none' | 'unchanged' | 'added' | 'removed' | 'changed';

export interface AnalysisInfo {
  pluginVersionId: string;
  status: AnalysisStatus;
  statusInfo: string;
  analyzed: string; // ISO date, empty until completed
  imageDigest: string;
  platform: string;
  // Every platform the image index offers; empty for a single-platform image.
  platforms: string[];
  filesTruncated: boolean;
}

export interface ChangeSummary {
  unchanged: number;
  added: number;
  removed: number;
  changed: number;
}

export interface ImagePackageEntry {
  name: string;
  // Empty for a removed package, which carries baseVersion instead.
  version: string;
  type: string;
  purl: string;
  licenses: string[];
  change: ChangeKind;
  baseVersion: string;
}

export interface ImageFileEntry {
  path: string;
  size: number;
  mode: number;
  link: string;
  change: ChangeKind;
}

export interface ImageAnalysisPage {
  analysis: AnalysisInfo;
  // Absent when no base was asked for.
  base?: AnalysisInfo;
  compared: boolean;
  // Every package; the page filters them itself.
  packages: ImagePackageEntry[];
  // One page of the filtered files.
  files: ImageFileEntry[];
  nextFilesPageToken: string;
  packageSummary: ChangeSummary;
  fileSummary: ChangeSummary;
  filesMatching: number;
}

export interface ImageAnalysisQuery {
  pluginVersionId: string;
  basePluginVersionId?: string;
  filesChangesOnly: boolean;
  filesQuery: string;
  filesPageToken?: string;
}

// An analysis that is still on its way: the page polls while one side is.
export function isInProgress(status: AnalysisStatus | undefined): boolean {
  return status === 'pending' || status === 'running' || status === 'retrying';
}

export function fromProtoAnalysisStatus(status: ImageAnalysisStatus): AnalysisStatus {
  switch (status) {
    case ImageAnalysisStatus.UNSPECIFIED:
      return 'none';
    case ImageAnalysisStatus.PENDING:
      return 'pending';
    case ImageAnalysisStatus.RUNNING:
      return 'running';
    case ImageAnalysisStatus.RETRYING:
      return 'retrying';
    case ImageAnalysisStatus.COMPLETED:
      return 'completed';
    case ImageAnalysisStatus.FAILED:
      return 'failed';
    default:
      throw new Error(`unhandled image analysis status: ${status satisfies never}`);
  }
}

export function fromProtoChange(change: Change): ChangeKind {
  switch (change) {
    case Change.UNSPECIFIED:
      return 'none';
    case Change.UNCHANGED:
      return 'unchanged';
    case Change.ADDED:
      return 'added';
    case Change.REMOVED:
      return 'removed';
    case Change.CHANGED:
      return 'changed';
    default:
      throw new Error(`unhandled change: ${change satisfies never}`);
  }
}

function fromProtoAnalysis(analysis: ProtoImageAnalysis): AnalysisInfo {
  return {
    pluginVersionId: analysis.pluginVersionId,
    status: fromProtoAnalysisStatus(analysis.status),
    statusInfo: analysis.statusInfo,
    analyzed: toIsoDate(analysis.analyzed),
    imageDigest: analysis.imageDigest,
    platform: analysis.platform,
    platforms: analysis.platforms,
    filesTruncated: analysis.filesTruncated,
  };
}

function fromProtoSummary(summary: ProtoChangeSummary | undefined): ChangeSummary {
  return {
    unchanged: summary?.unchanged ?? 0,
    added: summary?.added ?? 0,
    removed: summary?.removed ?? 0,
    changed: summary?.changed ?? 0,
  };
}

export function fromProtoImageAnalysis(
  response: GetImageAnalysisResponse,
  pluginVersionId: string,
): ImageAnalysisPage {
  return {
    analysis: response.analysis
      ? fromProtoAnalysis(response.analysis)
      : {
          pluginVersionId,
          status: 'none',
          statusInfo: '',
          analyzed: '',
          imageDigest: '',
          platform: '',
          platforms: [],
          filesTruncated: false,
        },
    base: response.base ? fromProtoAnalysis(response.base) : undefined,
    compared: response.compared,
    packages: response.packages.map((pkg) => ({
      name: pkg.name,
      version: pkg.version,
      type: pkg.type,
      purl: pkg.purl,
      licenses: pkg.licenses,
      change: fromProtoChange(pkg.change),
      baseVersion: pkg.baseVersion,
    })),
    files: response.files.map((file) => ({
      path: file.path,
      size: Number(file.size),
      mode: file.mode,
      link: file.link,
      change: fromProtoChange(file.change),
    })),
    nextFilesPageToken: response.nextFilesPageToken,
    packageSummary: fromProtoSummary(response.packageSummary),
    fileSummary: fromProtoSummary(response.fileSummary),
    filesMatching: response.filesMatching,
  };
}

// "12 added, 3 changed, 1 removed", leaving out the kinds that did not occur.
export function summaryText(summary: ChangeSummary): string {
  const parts = [
    summary.added ? `${summary.added} added` : '',
    summary.changed ? `${summary.changed} changed` : '',
    summary.removed ? `${summary.removed} removed` : '',
  ].filter((part) => part !== '');
  return parts.length > 0 ? parts.join(', ') : 'No changes';
}

export function changeLabel(change: ChangeKind): string {
  switch (change) {
    case 'none':
      return '';
    case 'unchanged':
      return 'Unchanged';
    case 'added':
      return 'Added';
    case 'removed':
      return 'Removed';
    case 'changed':
      return 'Changed';
    default:
      throw new Error(`unhandled change: ${change satisfies never}`);
  }
}

export function changeTagColor(change: ChangeKind): 'success' | 'critical' | 'warning' | 'neutral' {
  switch (change) {
    case 'added':
      return 'success';
    case 'removed':
      return 'critical';
    case 'changed':
      return 'warning';
    case 'unchanged':
    case 'none':
      return 'neutral';
    default:
      throw new Error(`unhandled change: ${change satisfies never}`);
  }
}

// 0755 style, as ls and chmod show permission bits.
export function formatMode(mode: number): string {
  return mode.toString(8).padStart(4, '0');
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ['KiB', 'MiB', 'GiB', 'TiB'];
  let value = bytes / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(1)} ${units[unit]}`;
}
