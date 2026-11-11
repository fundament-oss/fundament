import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';

import PluginInstallWatchService, { type InstallWatchTarget } from './install-watch.service';
import PluginInstallationService from './plugin-installation.service';
import { NotificationService } from '../notification.service';
import type { PluginInstallationItem } from '../plugin-resources/types';

const target: InstallWatchTarget = {
  clusterId: 'cl-1',
  clusterName: 'production',
  organizationName: 'acme',
  pluginName: 'cert-manager',
  pluginLabel: 'Cert Manager',
};

function withPhase(phase: string, message?: string): PluginInstallationItem {
  return {
    metadata: { name: 'acme--cert-manager', uid: 'uid-1' },
    spec: {
      definitionRef: {
        organizationName: 'acme',
        pluginName: 'cert-manager',
        pluginVersion: 'v1.0.0',
        definitionHash: 'sha256:abc',
      },
    },
    status: { phase, ready: phase === 'Running', message },
  };
}

// The service's pace while an installation is on its way.
const POLL_MS = 5000;

function build() {
  const getInstallation = vi.fn<PluginInstallationService['getInstallation']>();
  const notifications = { success: vi.fn(), warning: vi.fn(), error: vi.fn() };
  TestBed.configureTestingModule({
    providers: [
      { provide: PluginInstallationService, useValue: { getInstallation } },
      { provide: NotificationService, useValue: notifications },
    ],
  });
  return { watcher: TestBed.inject(PluginInstallWatchService), getInstallation, notifications };
}

describe('PluginInstallWatchService', () => {
  beforeEach(() => vi.useFakeTimers());

  afterEach(() => vi.useRealTimers());

  it('announces how an install ends, then stops reading', async () => {
    const { watcher, getInstallation, notifications } = build();
    getInstallation.mockResolvedValueOnce(withPhase('Deploying'));
    getInstallation.mockResolvedValueOnce(withPhase('Running'));

    watcher.watch(target, 'install');
    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(notifications.success).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(notifications.success).toHaveBeenCalledWith(
      'Plugin Cert Manager installed on cluster production',
    );

    await vi.advanceTimersByTimeAsync(POLL_MS * 3);
    expect(getInstallation).toHaveBeenCalledTimes(2);
  });

  it('leaves what a running plugin says out of the installed notification', async () => {
    const { watcher, getInstallation, notifications } = build();
    getInstallation.mockResolvedValue(withPhase('Running', 'all controllers ready'));

    watcher.watch(target, 'install');
    await vi.advanceTimersByTimeAsync(POLL_MS);

    expect(notifications.success).toHaveBeenCalledExactlyOnceWith(
      'Plugin Cert Manager installed on cluster production',
    );
  });

  it('announces a failed install as an error', async () => {
    const { watcher, getInstallation, notifications } = build();
    getInstallation.mockResolvedValue(withPhase('Failed', 'validate config: replicas too low'));

    watcher.watch(target, 'install');
    await vi.advanceTimersByTimeAsync(POLL_MS);

    expect(notifications.error).toHaveBeenCalledWith(
      'Failed to install plugin Cert Manager on cluster production',
      'The installation reports: “validate config: replicas too low”',
    );
  });

  it('announces the end of an uninstall once the installation is gone', async () => {
    const { watcher, getInstallation, notifications } = build();
    getInstallation.mockResolvedValueOnce(withPhase('Terminating'));
    getInstallation.mockResolvedValueOnce(null);

    watcher.watch(target, 'uninstall');
    await vi.advanceTimersByTimeAsync(POLL_MS * 2);

    expect(notifications.success).toHaveBeenCalledExactlyOnceWith(
      'Plugin Cert Manager removed from cluster production',
    );
  });

  it('warns about an install that came up degraded', async () => {
    const { watcher, getInstallation, notifications } = build();
    getInstallation.mockResolvedValueOnce(withPhase('Deploying'));
    getInstallation.mockResolvedValueOnce(
      withPhase('Degraded', 'container cert-manager: ImagePullBackOff: manifest unknown'),
    );

    watcher.watch(target, 'install');
    await vi.advanceTimersByTimeAsync(POLL_MS * 2);

    expect(notifications.warning).toHaveBeenCalledExactlyOnceWith(
      'Plugin Cert Manager is degraded on cluster production',
      'The installation reports: “container cert-manager: ImagePullBackOff: manifest unknown”',
    );
    expect(notifications.error).not.toHaveBeenCalled();
  });

  it('waits for an install that is not listed yet', async () => {
    const { watcher, getInstallation, notifications } = build();
    getInstallation.mockResolvedValueOnce(null);
    getInstallation.mockResolvedValueOnce(withPhase('Running'));

    watcher.watch(target, 'install');
    await vi.advanceTimersByTimeAsync(POLL_MS * 2);

    expect(notifications.success).toHaveBeenCalledTimes(1);
  });

  it('says nothing when an install it follows is removed by someone else', async () => {
    const { watcher, getInstallation, notifications } = build();
    getInstallation.mockResolvedValueOnce(withPhase('Deploying'));
    getInstallation.mockResolvedValue(null);

    watcher.watch(target, 'install');
    await vi.advanceTimersByTimeAsync(POLL_MS * 3);

    expect(notifications.success).not.toHaveBeenCalled();
    expect(getInstallation).toHaveBeenCalledTimes(2);
  });

  // Until the controller gets to the deletion, the installation still reports
  // the phase it had.
  it('keeps following an uninstall that still reads as running', async () => {
    const { watcher, getInstallation, notifications } = build();
    getInstallation.mockResolvedValueOnce(withPhase('Running'));
    getInstallation.mockResolvedValueOnce(null);

    watcher.watch(target, 'uninstall');
    await vi.advanceTimersByTimeAsync(POLL_MS);
    watcher.observe(target, 'Terminating', 'Running', watcher.readToken());
    await vi.advanceTimersByTimeAsync(POLL_MS);

    expect(notifications.success).toHaveBeenCalledExactlyOnceWith(
      'Plugin Cert Manager removed from cluster production',
    );
  });

  it('keeps reading through a failed read', async () => {
    const { watcher, getInstallation, notifications } = build();
    getInstallation.mockRejectedValueOnce(new Error('HTTP 503'));
    getInstallation.mockResolvedValueOnce(withPhase('Running'));

    watcher.watch(target, 'install');
    await vi.advanceTimersByTimeAsync(POLL_MS * 2);

    expect(notifications.success).toHaveBeenCalledTimes(1);
  });

  // A screen and the service read the same change a few seconds apart.
  it('announces a change a screen also read only once', async () => {
    const { watcher, getInstallation, notifications } = build();
    getInstallation.mockResolvedValue(withPhase('Running'));

    watcher.watch(target, 'install');
    await vi.advanceTimersByTimeAsync(POLL_MS);
    watcher.observe(target, 'Pending', 'Running', watcher.readToken());

    expect(notifications.success).toHaveBeenCalledTimes(1);
  });

  it('stops reading once a screen read the outcome first', async () => {
    const { watcher, getInstallation, notifications } = build();

    watcher.watch(target, 'install');
    watcher.observe(target, 'Pending', 'Running', watcher.readToken());
    await vi.advanceTimersByTimeAsync(POLL_MS * 2);

    expect(notifications.success).toHaveBeenCalledTimes(1);
    expect(getInstallation).not.toHaveBeenCalled();
  });

  it('announces a reinstall even right after the previous install was announced', async () => {
    const { watcher, notifications } = build();

    watcher.watch(target, 'install');
    watcher.observe(target, 'Pending', 'Running', watcher.readToken());
    watcher.watch(target, 'install');
    watcher.observe(target, 'Pending', 'Running', watcher.readToken());

    expect(notifications.success).toHaveBeenCalledTimes(2);
  });

  // A screen's read that was out when the user asked for something else.
  it('ignores what a screen read before the work it follows was asked for', async () => {
    const { watcher, getInstallation, notifications } = build();
    getInstallation.mockResolvedValue(null);

    const staleToken = watcher.readToken();
    watcher.watch(target, 'uninstall');
    watcher.observe(target, 'Pending', 'Running', staleToken);
    expect(notifications.success).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(notifications.success).toHaveBeenCalledExactlyOnceWith(
      'Plugin Cert Manager removed from cluster production',
    );
  });

  it('says nothing about the old installation while a retry replaces it', async () => {
    const { watcher, getInstallation, notifications } = build();
    getInstallation.mockResolvedValue(withPhase('Running'));
    let accept = () => {};
    const accepted = new Promise<void>((resolve) => {
      accept = resolve;
    });

    const retry = watcher.followRetry(target, () => accepted);
    watcher.observe(target, 'Pending', 'Failed', watcher.readToken(), 'parse manifest: bad yaml');
    watcher.observe(target, 'Failed', null, watcher.readToken());
    await vi.advanceTimersByTimeAsync(POLL_MS);

    expect(notifications.error).not.toHaveBeenCalled();
    expect(notifications.success).not.toHaveBeenCalled();
    expect(getInstallation).not.toHaveBeenCalled();

    accept();
    await retry;
    await vi.advanceTimersByTimeAsync(POLL_MS);

    expect(notifications.success).toHaveBeenCalledExactlyOnceWith(
      'Plugin Cert Manager installed on cluster production',
    );
  });

  // A watch can outlive the failure a retry answers: a screen's read from before
  // the watch started does not end it.
  it('does not announce what its own poll reads while a retry replaces the installation', async () => {
    const { watcher, getInstallation, notifications } = build();
    getInstallation.mockResolvedValueOnce(withPhase('Deploying'));
    getInstallation.mockResolvedValueOnce(withPhase('Failed', 'parse manifest: bad yaml'));
    getInstallation.mockResolvedValue(withPhase('Running'));
    let accept = () => {};
    const accepted = new Promise<void>((resolve) => {
      accept = resolve;
    });

    watcher.watch(target, 'install');
    await vi.advanceTimersByTimeAsync(POLL_MS);
    const retry = watcher.followRetry(target, () => accepted);
    await vi.advanceTimersByTimeAsync(POLL_MS);

    expect(getInstallation).toHaveBeenCalledTimes(2);
    expect(notifications.error).not.toHaveBeenCalled();

    accept();
    await retry;
    await vi.advanceTimersByTimeAsync(POLL_MS);

    expect(notifications.error).not.toHaveBeenCalled();
    expect(notifications.success).toHaveBeenCalledExactlyOnceWith(
      'Plugin Cert Manager installed on cluster production',
    );
  });

  it('follows nothing after a retry that was refused, and hears what screens read again', async () => {
    const { watcher, getInstallation, notifications } = build();

    await expect(
      watcher.followRetry(target, () => Promise.reject(new Error('aborted'))),
    ).rejects.toThrow('aborted');
    await vi.advanceTimersByTimeAsync(POLL_MS);
    expect(getInstallation).not.toHaveBeenCalled();

    watcher.observe(target, 'Failed', 'Running', watcher.readToken());
    expect(notifications.success).toHaveBeenCalledTimes(1);
  });

  it('announces what a screen reads for an installation nobody is waiting on', () => {
    const { watcher, notifications } = build();

    watcher.observe(target, 'Running', 'Failed', watcher.readToken(), 'parse manifest: bad yaml');

    expect(notifications.error).toHaveBeenCalledWith(
      'Plugin Cert Manager failed on cluster production',
      'The installation reports: “parse manifest: bad yaml”',
    );
  });
});
