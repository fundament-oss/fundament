import { Injectable, inject } from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { timestampFromDate, timestampDate } from '@bufbuild/protobuf/wkt';
import { firstValueFrom, map, type Observable } from 'rxjs';
import { CLUSTER, LOGS } from '../../connect/tokens';
import { ListClustersRequestSchema } from '../../generated/v1/cluster_pb';
import {
  QueryLogsRequestSchema,
  TailLogsRequestSchema,
  GetLogLabelsRequestSchema,
  LogBackend,
  LogSource,
  type LogEntry as ProtoLogEntry,
} from '../../generated/v1/logs_pb';
import type { LogEntry, LogLevel } from './log.types';

export interface ClusterOption {
  id: string;
  name: string;
}

export interface LogQuery {
  clusterId: string;
  namespace?: string;
  pod?: string;
  container?: string;
  search?: string;
  /**
   * Severities to return. Applied by the backend, not here: the entry limit
   * selects the newest matching lines, so filtering in the client meant a
   * namespace logging mostly INFO filled the page and a filter for ERROR
   * reported nothing while the errors sat just outside it.
   *
   * Omit, or pass every level, for "all".
   */
  levels?: readonly LogLevel[];
  /** Which of the cluster's log sources to read. */
  source?: LogSource;
  from?: Date;
  to?: Date;
  limit?: number;
}

export interface LogQueryResult {
  entries: LogEntry[];
  backend: LogBackend;
}

export interface LogLabels {
  namespaces: string[];
  pods: string[];
  containers: string[];
  backend: LogBackend;
}

const VALID_LEVELS: ReadonlySet<string> = new Set(['ERROR', 'WARN', 'INFO', 'DEBUG']);

function toViewLevel(level: string): LogLevel {
  return VALID_LEVELS.has(level) ? (level as LogLevel) : 'INFO';
}

/**
 * Normalises the level selection for the wire. An empty list and a full
 * selection both mean "all levels", and the backend reads an empty list that
 * way, so sending nothing is the cheaper equivalent.
 */
function levelsForRequest(levels: readonly LogLevel[] | undefined): string[] {
  if (!levels || levels.length === 0 || levels.length === VALID_LEVELS.size) {
    return [];
  }
  return [...levels];
}

/** Maps a backend LogEntry onto the frontend view model. */
export function mapLogEntry(proto: ProtoLogEntry, id: string): LogEntry {
  return {
    id,
    timestamp: proto.timestamp ? timestampDate(proto.timestamp) : new Date(),
    level: toViewLevel(proto.level),
    cluster: proto.cluster,
    namespace: proto.namespace,
    pod: proto.pod,
    container: proto.container,
    message: proto.message,
    fields: { ...proto.fields },
  };
}

/**
 * LogsApiService wraps the LogsService Connect client: fetching the cluster
 * list, querying historical logs, and live-tailing. The backend reports which
 * source answered via LogBackend, and applies the severity filter itself.
 */
@Injectable({ providedIn: 'root' })
export class LogsApiService {
  private readonly logsClient = inject(LOGS);

  private readonly clusterClient = inject(CLUSTER);

  async listClusters(): Promise<ClusterOption[]> {
    const response = await firstValueFrom(
      this.clusterClient.listClusters(create(ListClustersRequestSchema, {})),
    );
    return response.clusters.map((c) => ({ id: c.id, name: c.name }));
  }

  async query(q: LogQuery): Promise<LogQueryResult> {
    const response = await firstValueFrom(
      this.logsClient.queryLogs(
        create(QueryLogsRequestSchema, {
          clusterId: q.clusterId,
          namespace: q.namespace ?? '',
          pod: q.pod ?? '',
          container: q.container ?? '',
          search: q.search ?? '',
          levels: levelsForRequest(q.levels),
          source: q.source ?? LogSource.CLUSTER,
          start: q.from ? timestampFromDate(q.from) : undefined,
          end: q.to ? timestampFromDate(q.to) : undefined,
          limit: q.limit ?? 0,
        }),
      ),
    );
    return {
      entries: response.entries.map((e, i) => mapLogEntry(e, `log-${i}-${e.timestamp?.seconds}`)),
      backend: response.backend,
    };
  }

  async labels(clusterId: string, namespace?: string, from?: Date, to?: Date): Promise<LogLabels> {
    const response = await firstValueFrom(
      this.logsClient.getLogLabels(
        create(GetLogLabelsRequestSchema, {
          clusterId,
          namespace: namespace ?? '',
          start: from ? timestampFromDate(from) : undefined,
          end: to ? timestampFromDate(to) : undefined,
        }),
      ),
    );
    return {
      namespaces: response.namespaces,
      pods: response.pods,
      containers: response.containers,
      backend: response.backend,
    };
  }

  /** Streams new log entries until the subscription is closed. */
  tail(q: LogQuery): Observable<LogEntry> {
    let seq = 0;
    return this.logsClient
      .tailLogs(
        create(TailLogsRequestSchema, {
          clusterId: q.clusterId,
          namespace: q.namespace ?? '',
          pod: q.pod ?? '',
          container: q.container ?? '',
          search: q.search ?? '',
          levels: levelsForRequest(q.levels),
          source: q.source ?? LogSource.CLUSTER,
        }),
      )
      .pipe(
        map((e) => {
          seq += 1;
          return mapLogEntry(e, `live-${Date.now()}-${seq}`);
        }),
      );
  }
}
