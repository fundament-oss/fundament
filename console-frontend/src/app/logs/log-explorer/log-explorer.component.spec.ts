import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import LogExplorerComponent from './log-explorer.component';
import { LogsApiService } from '../logs.service';
import { ShootPodsService } from '../shoot-pods.service';
import { TitleService } from '../../title.service';
import PageNavService from '../../page-nav.service';
import { ConfigService } from '../../config.service';
import PluginInstallationService from '../../plugin-installation/plugin-installation.service';
import { Action } from '../../../generated/v1/actions_pb';
import { PermissionDeniedDetailSchema, RequiredRole } from '../../../generated/v1/authz_pb';

const DENIED = 'Logs are available to project members';

function denial(): ConnectError {
  return new ConnectError('denied', Code.PermissionDenied, undefined, [
    {
      desc: PermissionDeniedDetailSchema,
      value: create(PermissionDeniedDetailSchema, {
        action: Action.CLUSTER_CAN_VIEW_LOGS,
        requiredRole: RequiredRole.PROJECT_MEMBER,
      }),
    },
  ]);
}

async function build(listClusters: () => Promise<never>) {
  TestBed.configureTestingModule({
    providers: [
      { provide: TitleService, useValue: { setTitle: () => {} } },
      { provide: PageNavService, useValue: { backText: () => null } },
      { provide: ConfigService, useValue: { getConfig: () => ({}) } },
      { provide: LogsApiService, useValue: { listClusters } },
      { provide: ShootPodsService, useValue: {} },
      { provide: PluginInstallationService, useValue: {} },
    ],
  });
  const fixture = TestBed.createComponent(LogExplorerComponent);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('LogExplorerComponent denial', () => {
  it('shows the sentence in place of the page content', async () => {
    const el = await build(() => Promise.reject(denial()));
    expect(el.querySelector('nldd-inline-dialog')?.getAttribute('text')).toBe(DENIED);
    expect(el.querySelector('nldd-sidebar-section')).toBeNull();
  });

  it('keeps the banner and the content for other failures', async () => {
    const el = await build(() => Promise.reject(new ConnectError('boom', Code.Internal)));
    expect(el.querySelector('nldd-banner')?.getAttribute('text')).toBe('Failed to load logs');
    expect(el.querySelector('nldd-sidebar-section')).not.toBeNull();
  });
});
