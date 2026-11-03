import {
  hasInvalidConfig,
  installIssue,
  installPollDelay,
  installTransitionNotice,
} from './plugin-install-status';
import type { PluginInstallationItem } from '../plugin-resources/types';

function item(conditions?: { type: string; status: string }[]): PluginInstallationItem {
  return {
    metadata: { name: 'acme--ceph-rook', uid: 'uid-1' },
    spec: {
      definitionRef: {
        organizationName: 'acme',
        pluginName: 'ceph-rook',
        pluginVersion: 'v0.2.0',
        definitionHash: 'sha256:abc',
      },
    },
    status: { phase: 'Failed', ready: false, conditions },
  };
}

describe('hasInvalidConfig', () => {
  it('is true for a ConfigValid=False condition', () => {
    expect(hasInvalidConfig(item([{ type: 'ConfigValid', status: 'False' }]))).toBe(true);
  });

  it('is false when ConfigValid is True', () => {
    expect(hasInvalidConfig(item([{ type: 'ConfigValid', status: 'True' }]))).toBe(false);
  });

  it('is false for other failed conditions', () => {
    expect(hasInvalidConfig(item([{ type: 'PluginScopeReady', status: 'False' }]))).toBe(false);
  });

  it('is false when the CR carries no conditions', () => {
    expect(hasInvalidConfig(item())).toBe(false);
  });
});

describe('installIssue', () => {
  it('is null for a running, ready installation', () => {
    expect(installIssue({ phase: 'Running', ready: true, message: '' })).toBeNull();
  });

  it('does not show a running installation that is not ready as installed', () => {
    expect(installIssue({ phase: 'Running', ready: false, message: 'probe failing' })).toEqual({
      label: 'Not ready',
      badgeColor: 'warning',
      inProgress: false,
      message: 'probe failing',
    });
  });
});

describe('installPollDelay', () => {
  it('polls quickly while one is on its way', () => {
    expect(
      installPollDelay([
        { phase: 'Failed', configInvalid: false },
        { phase: 'Deploying', configInvalid: false },
      ]),
    ).toBe(5000);
  });

  it('polls slower while one is only unhealthy', () => {
    expect(installPollDelay([{ phase: 'Degraded', configInvalid: false }])).toBe(20000);
  });

  it('does not poll a failure its config caused, or a settled install', () => {
    expect(
      installPollDelay([
        { phase: 'Failed', configInvalid: true },
        { phase: 'Running', configInvalid: false },
      ]),
    ).toBeNull();
  });
});

describe('installTransitionNotice', () => {
  it('says a degraded plugin is running again', () => {
    expect(installTransitionNotice('Degraded', 'Running', 'Ceph', 'c1')?.text).toBe(
      'Plugin Ceph is running again on cluster c1',
    );
  });

  it('tells a failed install from a plugin that stopped working', () => {
    expect(installTransitionNotice('Deploying', 'Failed', 'Ceph', 'c1')?.text).toBe(
      'Failed to install plugin Ceph on cluster c1',
    );
    expect(installTransitionNotice('Running', 'Failed', 'Ceph', 'c1')?.text).toBe(
      'Plugin Ceph failed on cluster c1',
    );
  });

  it('says nothing for a change that needs no notice', () => {
    expect(installTransitionNotice('Running', 'Degraded', 'Ceph', 'c1')).toBeNull();
  });
});
