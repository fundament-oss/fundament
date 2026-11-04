import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import LogAnalyticsComponent from './log-analytics.component';
import { LogsApiService } from '../logs.service';
import { TitleService } from '../../title.service';
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
      { provide: LogsApiService, useValue: { listClusters } },
    ],
  });
  const fixture = TestBed.createComponent(LogAnalyticsComponent);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('LogAnalyticsComponent denial', () => {
  it('shows the sentence in place of the page content', async () => {
    const el = await build(() => Promise.reject(denial()));
    expect(el.textContent).toContain(DENIED);
    expect(el.querySelector('select')).toBeNull();
    expect(el.textContent).not.toContain('Total logs');
  });

  it('keeps the inline error and the content for other failures', async () => {
    const el = await build(() => Promise.reject(new ConnectError('boom', Code.Internal)));
    expect(el.textContent).toContain('Failed to load logs. Please try again.');
    expect(el.textContent).toContain('Total logs');
  });
});
