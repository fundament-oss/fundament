import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError, createRouterTransport } from '@connectrpc/connect';
import { AUTHZ_TRANSPORT } from '../connect/connect.module';
import { Action } from '../generated/v1/actions_pb';
import { AuthzService, CheckPermissionsResponseSchema, PermissionCheckSchema, type PermissionCheck } from '../generated/v1/authz_pb';
import PermissionsService from './permissions.service';

const check = (i: number): PermissionCheck =>
  create(PermissionCheckSchema, { action: Action.CLUSTER_CAN_VIEW, objectId: String(i) });

function provide(handler: (ids: string[]) => boolean[]) {
  const sizes: number[] = [];
  const transport = createRouterTransport(({ service }) => {
    service(AuthzService, {
      checkPermissions: (req) => {
        sizes.push(req.checks.length);
        return create(CheckPermissionsResponseSchema, {
          allowed: handler(req.checks.map((c) => c.objectId)),
        });
      },
    });
  });
  TestBed.configureTestingModule({
    providers: [{ provide: AUTHZ_TRANSPORT, useValue: transport }],
  });
  return sizes;
}

describe('PermissionsService', () => {
  it('keeps request order across a split at 500', async () => {
    const sizes = provide((ids) => ids.map((id) => Number(id) % 2 === 0));
    const checks = Array.from({ length: 501 }, (_, i) => check(i));
    const result = await TestBed.inject(PermissionsService).can(checks);
    expect(sizes).toEqual([500, 1]);
    expect(result).toEqual(checks.map((_, i) => i % 2 === 0));
  });

  it('returns an empty list without calling for no checks', async () => {
    const sizes = provide(() => []);
    expect(await TestBed.inject(PermissionsService).can([])).toEqual([]);
    expect(sizes).toEqual([]);
  });

  it('returns all false and logs when the reply length does not match', async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    provide((ids) => ids.slice(1).map(() => true));
    const result = await TestBed.inject(PermissionsService).can([check(1), check(2), check(3)]);
    expect(result).toEqual([false, false, false]);
    expect(error).toHaveBeenCalled();
    error.mockRestore();
  });

  it('returns all false and logs when the transport fails', async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const transport = createRouterTransport(({ service }) => {
      service(AuthzService, {
        checkPermissions: () => {
          throw new ConnectError('down', Code.Unavailable);
        },
      });
    });
    TestBed.configureTestingModule({
      providers: [{ provide: AUTHZ_TRANSPORT, useValue: transport }],
    });
    const result = await TestBed.inject(PermissionsService).can([check(1), check(2)]);
    expect(result).toEqual([false, false]);
    expect(error).toHaveBeenCalled();
    error.mockRestore();
  });
});
