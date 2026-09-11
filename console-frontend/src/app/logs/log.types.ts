export type LogLevel = 'ERROR' | 'WARN' | 'INFO' | 'DEBUG';

export interface LogEntry {
  id: string;
  timestamp: Date;
  level: LogLevel;
  cluster: string;
  namespace: string;
  pod: string;
  container: string;
  message: string;
  fields: Record<string, unknown>;
}

/**
 * One bucket of the log histogram, as counted by the backend. `start` is the
 * bucket's own timestamp rather than a formatted label: the axis label depends
 * on how wide the selected window is, which is the chart's business, not the
 * data's.
 */
export interface HistogramBucket {
  start: Date;
  error: number;
  warn: number;
  info: number;
  debug: number;
}
