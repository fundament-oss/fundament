import { TestBed } from '@angular/core/testing';
import { NEVER, throwError } from 'rxjs';
import { Code, ConnectError } from '@connectrpc/connect';
import { vi } from 'vitest';
import LogExplorerComponent, { containersOf } from './log-explorer.component';
import { LogsApiService, type LogHistogram, type LogQueryResult } from '../logs.service';
import { ShootPodsService } from '../shoot-pods.service';
import type { HistogramBucket, LogLevel } from '../log.types';
import { LogBackend, LogSource } from '../../../generated/v1/logs_pb';
import { TitleService } from '../../title.service';
import { NotificationService } from '../../notification.service';
import PageNavService from '../../page-nav.service';
import { ConfigService } from '../../config.service';
import PluginInstallationService from '../../plugin-installation/plugin-installation.service';

const BUCKETS: HistogramBucket[] = [
  { start: new Date('2026-11-16T09:00:00Z'), error: 2, warn: 3, info: 40, debug: 1 },
  { start: new Date('2026-11-16T09:30:00Z'), error: 1, warn: 0, info: 60, debug: 4 },
];

/** A promise the test settles by hand, to hold a response in flight. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

/** Lets every already-settled response land, the histogram after the query. */
const settle = () =>
  new Promise<void>((r) => {
    setTimeout(r);
  });

function build(opts: { backend?: LogBackend; exact?: boolean } = {}) {
  const backend = opts.backend ?? LogBackend.LOKI;
  const logsApi = {
    // No entries: the counts never read them, and the chart, which Chart.js
    // cannot draw without a canvas context, only renders above a list.
    query: vi.fn(async (): Promise<LogQueryResult> => ({ entries: [], backend })),
    histogram: vi.fn(async (): Promise<LogHistogram> => ({
      buckets: BUCKETS,
      backend,
      exact: opts.exact ?? true,
    })),
    tail: vi.fn(() => NEVER),
    labels: vi.fn(async () => ({ namespaces: [], pods: [], containers: [], backend })),
  };
  const notifications = { error: vi.fn(), info: vi.fn() };
  TestBed.configureTestingModule({
    providers: [
      { provide: LogsApiService, useValue: logsApi },
      { provide: TitleService, useValue: { setTitle: () => {} } },
      { provide: ConfigService, useValue: { getConfig: () => ({}) } },
      { provide: NotificationService, useValue: notifications },
      { provide: PageNavService, useValue: { backText: () => null } },
      { provide: PluginInstallationService, useValue: {} },
      { provide: ShootPodsService, useValue: {} },
    ],
  });
  // Not rendered: these are the signals the sidebar and chart read, driven
  // through the same methods the template calls.
  const component = TestBed.createComponent(LogExplorerComponent).componentInstance;
  component.selectedCluster.set('c1');
  const load = async () => {
    await component['loadLogs']();
    await settle();
  };
  return { component, logsApi, load, notifications };
}

describe('LogExplorerComponent level counts', () => {
  it('counts every level over the whole window, ticked or not', async () => {
    const { component, logsApi, load } = build();
    component.selectedLevels.set(new Set<LogLevel>(['ERROR']));
    await load();

    expect(logsApi.histogram).toHaveBeenCalledWith(expect.objectContaining({ levels: undefined }));
    expect(component.levelCounts()).toEqual({ ERROR: 3, WARN: 3, INFO: 100, DEBUG: 5 });
    expect(component.levelCountLabel('INFO')).toBe('100');
    expect(component.allLevelsCountLabel()).toBe('111');
  });

  it('draws only the ticked levels in the chart', async () => {
    const { component, load } = build();
    component.selectedLevels.set(new Set<LogLevel>(['ERROR', 'DEBUG']));
    await load();

    expect(component['chartBuckets']()).toEqual([
      { start: BUCKETS[0].start, error: 2, warn: 0, info: 0, debug: 1 },
      { start: BUCKETS[1].start, error: 1, warn: 0, info: 0, debug: 4 },
    ]);
  });

  it('shows no counts when the histogram only counted a page', async () => {
    const { component, load } = build({ exact: false });
    await load();

    expect(component.levelCounts()).toBeNull();
    expect(component.levelCountLabel('ERROR')).toBe('');
    expect(component.allLevelsCountLabel()).toBe('');
  });

  it('shows no counts on the pod-log fallback, which counts its own page', async () => {
    const { component, logsApi, load } = build({ backend: LogBackend.KUBERNETES });
    component.backend.set(LogBackend.KUBERNETES);
    component.selectedNamespace.set('ns');
    component.selectedPod.set('pod-a');
    await load();

    expect(logsApi.histogram).not.toHaveBeenCalled();
    expect(component.histogramExact()).toBe(false);
    expect(component.levelCounts()).toBeNull();
  });

  it('keeps the counts while a level change reloads, since they do not depend on it', async () => {
    const { component, logsApi, load } = build();
    await load();
    const pending = deferred<LogHistogram>();
    logsApi.histogram.mockReturnValueOnce(pending.promise);

    component.toggleLevel('ERROR');
    await settle();

    expect(logsApi.histogram).toHaveBeenCalledTimes(2);
    expect(component.levelCountLabel('INFO')).toBe('100');
  });

  it('blanks the counts as soon as the search changes, before the debounce reloads', async () => {
    const { component, logsApi, load } = build();
    await load();

    component.onSearchChange('timeout');

    expect(logsApi.query).toHaveBeenCalledTimes(1);
    expect(component.levelCounts()).toBeNull();
  });

  it('blanks the counts on a filter change until the answer for it lands', async () => {
    const { component, logsApi, load } = build();
    await load();
    const pending = deferred<LogHistogram>();
    logsApi.histogram.mockReturnValueOnce(pending.promise);

    component.onTimePresetChange('24h');
    await settle();
    expect(component.levelCounts()).toBeNull();

    pending.resolve({ buckets: BUCKETS, backend: LogBackend.LOKI, exact: true });
    await settle();
    expect(component.levelCountLabel('ERROR')).toBe('3');
  });

  it('drops an answer to a query the filters have since moved away from', async () => {
    const { component, logsApi, load } = build();
    const pending = deferred<LogHistogram>();
    logsApi.histogram.mockReturnValueOnce(pending.promise);
    await load();

    // Still the same load, so the sequence check lets the answer through; it
    // is the query it answered that no longer matches.
    component.searchText.set('timeout');
    pending.resolve({ buckets: BUCKETS, backend: LogBackend.LOKI, exact: true });
    await settle();

    expect(component.levelCounts()).toBeNull();
  });

  it('drops a live-tail poll that lands after a newer one', async () => {
    const { component, logsApi, load } = build();
    await load();
    const older = deferred<LogHistogram>();
    const newer = deferred<LogHistogram>();
    logsApi.histogram.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);

    // Two tail ticks under the same load, as `startLiveTail` sends them.
    const { from, to } = component['timeRange']();
    component['refreshHistogram'](from, to, component['loadSeq']);
    component['refreshHistogram'](from, to, component['loadSeq']);
    newer.resolve({ buckets: BUCKETS.slice(1), backend: LogBackend.LOKI, exact: true });
    await settle();
    older.resolve({ buckets: BUCKETS, backend: LogBackend.LOKI, exact: true });
    await settle();

    expect(component.histogramBuckets()).toEqual(BUCKETS.slice(1));
    expect(component.levelCountLabel('ERROR')).toBe('1');
  });
});

describe('LogExplorerComponent level selection', () => {
  it('selects every level again when the last one is unticked', () => {
    const { component } = build();
    component.selectedLevels.set(new Set<LogLevel>(['WARN']));

    component.toggleLevel('WARN');

    expect(component.isAllLevelsSelected()).toBe(true);
  });
});

describe('LogExplorerComponent plugin logs', () => {
  it('reads every pod of the plugin when no pod is picked', async () => {
    const { component, logsApi, load } = build({ backend: LogBackend.KUBERNETES });
    component.sourceMode.set('live');
    component.selectedNamespace.set('plugin-cert-manager');
    await load();

    expect(logsApi.query).toHaveBeenCalledWith(
      expect.objectContaining({
        namespace: 'plugin-cert-manager',
        pod: undefined,
        source: LogSource.PLUGIN,
      }),
    );
    expect(component.podLabel()).toBe('All pods');
  });

  it('does not query before a plugin is picked', async () => {
    const { component, logsApi, load } = build({ backend: LogBackend.KUBERNETES });
    component.sourceMode.set('live');
    await load();

    expect(logsApi.query).not.toHaveBeenCalled();
  });

  it('asks for a plugin instead of streaming without one', () => {
    const { component, logsApi, notifications } = build();
    component.sourceMode.set('live');

    component.setStreamState('streaming');

    expect(logsApi.tail).not.toHaveBeenCalled();
    expect(notifications.info).toHaveBeenCalledWith('Select a plugin first');
    expect(component.streamState()).toBe('off');
  });

  it('asks for a namespace on the pod-log fallback', () => {
    const { component, logsApi, notifications } = build({ backend: LogBackend.KUBERNETES });
    component.backend.set(LogBackend.KUBERNETES);

    component.setStreamState('streaming');

    expect(logsApi.tail).not.toHaveBeenCalled();
    expect(notifications.info).toHaveBeenCalledWith('Select a namespace first');
  });

  it('says why a refused stream stopped rather than that it disconnected', () => {
    const { component, logsApi, notifications } = build();
    logsApi.tail.mockReturnValueOnce(
      throwError(
        () => new ConnectError('kubernetes log backend requires a namespace', Code.InvalidArgument),
      ),
    );

    component.setStreamState('streaming');

    expect(notifications.error).toHaveBeenCalledWith('kubernetes log backend requires a namespace');
    expect(component.streamState()).toBe('off');
  });

  it('reports a dropped stream as disconnected', () => {
    const { component, logsApi, notifications } = build();
    logsApi.tail.mockReturnValueOnce(throwError(() => new Error('network')));

    component.setStreamState('streaming');

    expect(notifications.error).toHaveBeenCalledWith('Live tail disconnected');
  });

  it('offers every container of every pod, each once', () => {
    expect(
      containersOf([
        { name: 'web-0', containers: ['app', 'sidecar'] },
        { name: 'web-1', containers: ['app'] },
      ]),
    ).toEqual(['app', 'sidecar']);
  });
});
