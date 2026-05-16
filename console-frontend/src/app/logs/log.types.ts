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

export interface HistogramBucket {
  label: string;
  error: number;
  warn: number;
  info: number;
  debug: number;
}
