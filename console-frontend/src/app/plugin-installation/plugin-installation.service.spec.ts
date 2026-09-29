import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import PluginInstallationService, {
  pluginResourceName,
  RetryConfigVersionError,
  RetryReadError,
} from './plugin-installation.service';
import { ConfigService } from '../config.service';
import type { PluginInstallationItem } from '../plugin-resources/types';

describe('pluginResourceName', () => {
  it('qualifies the plugin name with its publishing organization', () => {
    expect(pluginResourceName('acme', 'cert-manager')).toBe('acme--cert-manager');
  });

  it('slugifies display-style names', () => {
    expect(pluginResourceName('Acme Corp', 'Grafana Alloy')).toBe('acme-corp--grafana-alloy');
  });

  it('keeps the separator when a half slugs to something containing a dash', () => {
    // Slugging the joined string would collapse "--" back to "-" and lose the
    // boundary, making ("acme", "corp-grafana") indistinguishable from
    // ("acme-corp", "grafana").
    expect(pluginResourceName('acme-corp', 'grafana')).toBe('acme-corp--grafana');
    expect(pluginResourceName('acme', 'corp-grafana')).toBe('acme--corp-grafana');
  });
});

describe('PluginInstallationService retryInstall', () => {
  function build(): PluginInstallationService {
    TestBed.configureTestingModule({
      providers: [
        {
          provide: ConfigService,
          useValue: { getConfig: () => ({ kubeApiProxyUrl: '' }) } as unknown as ConfigService,
        },
      ],
    });
    return TestBed.inject(PluginInstallationService);
  }

  const failedInstall = {
    metadata: { name: 'acme--ceph-rook', uid: 'uid-1' },
    spec: {
      definitionRef: {
        organizationName: 'acme',
        pluginName: 'ceph-rook',
        pluginVersion: 'v0.2.0',
        definitionHash: 'sha256:abc',
      },
      config: { MON_COUNT: '1', DEV_LOOP_DEVICES: 'true' },
    },
    status: { phase: 'Failed', ready: false },
  } as PluginInstallationItem;

  it('re-creates the installation with its previous config', async () => {
    const service = build();
    // First read returns the failed CR; the uninstall poll then sees it gone.
    vi.spyOn(service, 'getInstallation')
      .mockResolvedValueOnce(failedInstall)
      .mockResolvedValue(null);
    const uninstall = vi.spyOn(service, 'uninstallPlugin').mockResolvedValue(undefined);
    const install = vi.spyOn(service, 'installPlugin').mockResolvedValue(undefined);

    await service.retryInstall('c1', 'acme', 'ceph-rook', 'v0.2.0', 'sha256:abc');

    expect(uninstall).toHaveBeenCalledWith('c1', 'acme--ceph-rook');
    expect(install).toHaveBeenCalledWith('c1', 'acme', 'ceph-rook', 'v0.2.0', 'sha256:abc', {
      MON_COUNT: '1',
      DEV_LOOP_DEVICES: 'true',
    });
  });

  // Config was written against the recorded version's schema, so a retry that
  // switches versions (the unrecorded-pin fallback) must not replay it against
  // a different schema — and must abort before deleting the only copy.
  it('aborts with RetryConfigVersionError, without uninstalling, on a version switch with config', async () => {
    const service = build();
    vi.spyOn(service, 'getInstallation').mockResolvedValue(failedInstall);
    const uninstall = vi.spyOn(service, 'uninstallPlugin').mockResolvedValue(undefined);
    const install = vi.spyOn(service, 'installPlugin').mockResolvedValue(undefined);

    await expect(
      service.retryInstall('c1', 'acme', 'ceph-rook', 'v0.3.0', 'sha256:def'),
    ).rejects.toBeInstanceOf(RetryConfigVersionError);
    expect(uninstall).not.toHaveBeenCalled();
    expect(install).not.toHaveBeenCalled();
  });

  it('allows a version switch when the failed install carries no config', async () => {
    const service = build();
    const configless = {
      ...failedInstall,
      spec: { definitionRef: failedInstall.spec.definitionRef },
    } as PluginInstallationItem;
    vi.spyOn(service, 'getInstallation')
      .mockResolvedValueOnce(configless)
      .mockResolvedValue(null);
    vi.spyOn(service, 'uninstallPlugin').mockResolvedValue(undefined);
    const install = vi.spyOn(service, 'installPlugin').mockResolvedValue(undefined);

    await service.retryInstall('c1', 'acme', 'ceph-rook', 'v0.3.0', 'sha256:def');

    expect(install).toHaveBeenCalledWith('c1', 'acme', 'ceph-rook', 'v0.3.0', 'sha256:def', {});
  });

  // A read failure (network blip, RBAC hiccup) is not "the CR is already
  // gone": proceeding would uninstall the plugin's only copy of its config.
  it('aborts with RetryReadError, without uninstalling, when the read fails', async () => {
    const service = build();
    vi.spyOn(service, 'getInstallation').mockRejectedValue(new Error('HTTP 500'));
    const uninstall = vi.spyOn(service, 'uninstallPlugin').mockResolvedValue(undefined);
    const install = vi.spyOn(service, 'installPlugin').mockResolvedValue(undefined);

    await expect(
      service.retryInstall('c1', 'acme', 'ceph-rook', 'v0.2.0', 'sha256:abc'),
    ).rejects.toBeInstanceOf(RetryReadError);
    expect(uninstall).not.toHaveBeenCalled();
    expect(install).not.toHaveBeenCalled();
  });

  it('proceeds with empty config when the CR is already gone (404 resolves null)', async () => {
    const service = build();
    vi.spyOn(service, 'getInstallation').mockResolvedValue(null);
    vi.spyOn(service, 'uninstallPlugin').mockResolvedValue(undefined);
    vi.spyOn(service, 'listInstallations').mockResolvedValue([]);
    const install = vi.spyOn(service, 'installPlugin').mockResolvedValue(undefined);

    await service.retryInstall('c1', 'acme', 'ceph-rook', 'v0.2.0', 'sha256:abc');

    expect(install).toHaveBeenCalledWith('c1', 'acme', 'ceph-rook', 'v0.2.0', 'sha256:abc', {});
  });
});
