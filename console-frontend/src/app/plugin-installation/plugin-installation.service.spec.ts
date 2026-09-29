import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import PluginInstallationService, {
  pluginResourceName,
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

  it('re-creates the installation with its previous config', async () => {
    const service = build();
    vi.spyOn(service, 'getInstallation').mockResolvedValue({
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
    } as PluginInstallationItem);
    const uninstall = vi.spyOn(service, 'uninstallPlugin').mockResolvedValue(undefined);
    vi.spyOn(service, 'listInstallations').mockResolvedValue([]);
    const install = vi.spyOn(service, 'installPlugin').mockResolvedValue(undefined);

    await service.retryInstall('c1', 'acme', 'ceph-rook', 'v0.2.0', 'sha256:abc');

    expect(uninstall).toHaveBeenCalledWith('c1', 'acme--ceph-rook');
    expect(install).toHaveBeenCalledWith('c1', 'acme', 'ceph-rook', 'v0.2.0', 'sha256:abc', {
      MON_COUNT: '1',
      DEV_LOOP_DEVICES: 'true',
    });
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
