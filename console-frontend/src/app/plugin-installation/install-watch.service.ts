import { Injectable, inject } from '@angular/core';

import { NotificationService } from '../notification.service';
import {
  type InstallTransitionNotice,
  installPhase,
  installRemovedNotice,
  installTransitionNotice,
  isInstallInProgress,
} from '../utils/plugin-install-status';
import PluginInstallationService, { pluginResourceName } from './plugin-installation.service';

/** One plugin on one cluster, with the names a notification calls them by. */
export interface InstallWatchTarget {
  clusterId: string;
  clusterName: string;
  organizationName: string;
  pluginName: string;
  /** The plugin's display name. */
  pluginLabel: string;
}

/** Which way the work goes, which decides what an installation that is gone
 *  means: the end of an uninstall, or nothing to report on. */
export type InstallWatchDirection = 'install' | 'uninstall';

interface Watch extends InstallWatchTarget {
  direction: InstallWatchDirection;
  /** The phase last read, by this service or by a screen. */
  phase: string;
  startedAt: number;
}

/** The pace of installPollDelay for an installation on its way. */
const POLL_MS = 5000;

/** An install still on its way after this long is stuck rather than slow; the
 *  screens that show it keep saying so, but nobody is waiting for a
 *  notification any more. */
const GIVE_UP_MS = 30 * 60 * 1000;

/** How long an announced outcome stays announced: longer than any screen's
 *  poll takes to read the same change. */
const ANNOUNCED_MS = 60 * 1000;

/** The phase an announcement records for an installation that is gone. */
const GONE = '';

function keyOf(target: InstallWatchTarget): string {
  return `${target.clusterId}/${pluginResourceName(target.organizationName, target.pluginName)}`;
}

/**
 * Follows the installs, retries and uninstalls started in this console until
 * they are done, and says how each one ended, wherever the user has gone in
 * the meantime. The screens that started them stop reading when they are left,
 * so without this an install would finish unannounced as soon as the user
 * navigated away.
 *
 * A screen that shows installations reports what it reads through observe(),
 * rather than raising notifications itself. Both may read the same change, a
 * few seconds apart; this service announces it once.
 */
@Injectable({ providedIn: 'root' })
export default class PluginInstallWatchService {
  private installations = inject(PluginInstallationService);

  private notifications = inject(NotificationService);

  private watches = new Map<string, Watch>();

  /** The outcome last announced per installation, and when. */
  private announced = new Map<string, { phase: string; at: number }>();

  private timer: ReturnType<typeof setTimeout> | null = null;

  /** Starts following an installation the user just asked to change. Call it
   *  once the request was accepted: a rejected one changes nothing to wait
   *  for. */
  watch(target: InstallWatchTarget, direction: InstallWatchDirection): void {
    const key = keyOf(target);
    // A new piece of work: how the previous one on this installation ended
    // must not stop this one's outcome from being announced.
    this.announced.delete(key);
    this.watches.set(key, {
      ...target,
      direction,
      phase: direction === 'install' ? 'Pending' : 'Terminating',
      startedAt: Date.now(),
    });
    this.schedule();
  }

  /** Reports a change a screen read: the phase it had, and the phase it has
   *  now, or null once it is gone. Announces it unless that was done
   *  already. */
  observe(target: InstallWatchTarget, previous: string, next: string | null): void {
    const key = keyOf(target);
    const watch = this.watches.get(key);
    if (watch) {
      if (next === null || !isInstallInProgress(next)) this.watches.delete(key);
      else watch.phase = next;
    }
    this.announce(key, target, previous, next);
  }

  private schedule(): void {
    if (this.timer !== null || this.watches.size === 0) return;
    this.timer = setTimeout(() => {
      this.timer = null;
      this.poll()
        .catch(() => {})
        .finally(() => this.schedule());
    }, POLL_MS);
  }

  private async poll(): Promise<void> {
    const now = Date.now();
    await Promise.all(
      [...this.watches.entries()].map(async ([key, watch]) => {
        if (now - watch.startedAt > GIVE_UP_MS) {
          this.watches.delete(key);
          return;
        }
        let item;
        try {
          item = await this.installations.getInstallation(
            watch.clusterId,
            pluginResourceName(watch.organizationName, watch.pluginName),
          );
        } catch {
          return; // A failed read says nothing; the next one may.
        }
        // Ended or started over while the read was out: what it read belongs
        // to work that is no longer this watch's.
        if (this.watches.get(key) !== watch) return;

        if (item === null) {
          this.watches.delete(key);
          // Gone halfway through an install means someone else removed it;
          // only the end of an uninstall is this watch's outcome.
          if (watch.direction === 'uninstall') this.announce(key, watch, watch.phase, null);
          return;
        }
        const next = installPhase(item.status?.phase);
        if (isInstallInProgress(next)) this.watches.set(key, { ...watch, phase: next });
        else this.watches.delete(key);
        this.announce(key, watch, watch.phase, next);
      }),
    );
  }

  private announce(
    key: string,
    target: InstallWatchTarget,
    previous: string,
    next: string | null,
  ): void {
    if (previous === next) return;
    const now = Date.now();
    const phase = next ?? GONE;
    const last = this.announced.get(key);
    if (last && last.phase === phase && now - last.at < ANNOUNCED_MS) return;

    const notice: InstallTransitionNotice | null =
      next === null
        ? installRemovedNotice(target.pluginLabel, target.clusterName)
        : installTransitionNotice(previous, next, target.pluginLabel, target.clusterName);
    if (!notice) return;

    this.announced.forEach((entry, entryKey) => {
      if (now - entry.at >= ANNOUNCED_MS) this.announced.delete(entryKey);
    });
    this.announced.set(key, { phase, at: now });
    if (notice.kind === 'success') this.notifications.success(notice.text);
    else this.notifications.error(notice.text);
  }
}
