import { hasInvalidConfig } from './plugin-install-status';
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
