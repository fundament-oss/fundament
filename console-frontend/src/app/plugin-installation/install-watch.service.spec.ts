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

function withPhase(phase: string): PluginInstallationItem {
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
    status: { phase, ready: phase === 'Running' },
  };
}

// The service's pace while an installation is on its way.
const POLL_MS = 5000;

function build() {
  const getInstallation = vi.fn<PluginInstallationService['getInstallation']>();
  const notifications = { success: vi.fn(), error: vi.fn() };
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

  it('announces a failed install as an error', async () => {
    const { watcher, getInstallation, notifications } = build();
    getInstallation.mockResolvedValue(withPhase('Failed'));

    watcher.watch(target, 'install');
    await vi.advanceTimersByTimeAsync(POLL_MS);

    expect(notifications.error).toHaveBeenCalledWith(
      'Failed to install plugin Cert Manager on cluster production',
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

  it('says nothing when an install it follows is removed by someone else', async () => {
    const { watcher, getInstallation, notifications } = build();
    getInstallation.mockResolvedValue(null);

    watcher.watch(target, 'install');
    await vi.advanceTimersByTimeAsync(POLL_MS * 2);

    expect(notifications.success).not.toHaveBeenCalled();
    expect(getInstallation).toHaveBeenCalledTimes(1);
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
    watcher.observe(target, 'Pending', 'Running');

    expect(notifications.success).toHaveBeenCalledTimes(1);
  });

  it('stops reading once a screen read the outcome first', async () => {
    const { watcher, getInstallation, notifications } = build();

    watcher.watch(target, 'install');
    watcher.observe(target, 'Pending', 'Running');
    await vi.advanceTimersByTimeAsync(POLL_MS * 2);

    expect(notifications.success).toHaveBeenCalledTimes(1);
    expect(getInstallation).not.toHaveBeenCalled();
  });

  it('announces a reinstall even right after the previous install was announced', async () => {
    const { watcher, notifications } = build();

    watcher.watch(target, 'install');
    watcher.observe(target, 'Pending', 'Running');
    watcher.watch(target, 'install');
    watcher.observe(target, 'Pending', 'Running');

    expect(notifications.success).toHaveBeenCalledTimes(2);
  });

  it('announces what a screen reads for an installation nobody is waiting on', () => {
    const { watcher, notifications } = build();

    watcher.observe(target, 'Running', 'Failed');

    expect(notifications.error).toHaveBeenCalledWith(
      'Plugin Cert Manager failed on cluster production',
    );
  });
});
