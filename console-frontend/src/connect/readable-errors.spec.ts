import { Code, ConnectError, createRouterTransport } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';
import {
  INTERNAL_MESSAGE,
  UNAVAILABLE_MESSAGE,
  readableConnectError,
  readableErrors,
} from './readable-errors';
import {
  OrganizationService,
  ListOrganizationsResponseSchema,
} from '../generated/v1/organization_pb';
import { LogEntrySchema, LogsService } from '../generated/v1/logs_pb';
import { createObservableClient } from './observable-client';

describe('readableConnectError', () => {
  it('gives fixed copy when the API cannot be reached', () => {
    const outage = new ConnectError(
      'permissions could not be checked because the authorization service is unavailable; try again in a moment',
      Code.Unavailable,
    );
    expect(readableConnectError(outage).message).toBe(UNAVAILABLE_MESSAGE);
    expect(readableConnectError(new ConnectError('HTTP 503', Code.Unavailable)).message).toBe(
      UNAVAILABLE_MESSAGE,
    );
  });

  it('reads fetch failing outright as the API being unavailable', () => {
    const error = readableConnectError(new TypeError('Failed to fetch'));
    expect(error.code).toBe(Code.Unavailable);
    expect(error.message).toBe(UNAVAILABLE_MESSAGE);
  });

  it('keeps backend detail out of internal failures, but in the cause', () => {
    const internal = new ConnectError(
      'authorization check failed: check: Post "http://openfga:8080/stores": dial tcp: connection refused',
      Code.Internal,
    );
    const error = readableConnectError(internal);
    expect(error.message).toBe(INTERNAL_MESSAGE);
    expect(error.cause).toBe(internal);
    expect(readableConnectError(new ConnectError('boom', Code.Unknown)).message).toBe(
      INTERNAL_MESSAGE,
    );
  });

  it('passes a refusal meant for the reader through without the code prefix', () => {
    const taken = new ConnectError('a project with this name already exists', Code.AlreadyExists);
    expect(taken.message).toBe('[already_exists] a project with this name already exists');

    const error = readableConnectError(taken);
    expect(error).toBeInstanceOf(ConnectError);
    expect(error.code).toBe(Code.AlreadyExists);
    expect(error.message).toBe('a project with this name already exists');
    expect(error.rawMessage).toBe('a project with this name already exists');
  });
});

describe('readableErrors', () => {
  it('rewrites a call the server refused before the page sees it', async () => {
    const transport = createRouterTransport(
      ({ service }) => {
        service(OrganizationService, {
          listOrganizations: () => {
            throw new ConnectError('authorization check failed: dial tcp', Code.Internal);
          },
        });
      },
      { transport: { interceptors: [readableErrors] } },
    );
    const client = createObservableClient(OrganizationService, transport);

    await expect(
      new Promise((resolve, reject) => {
        client.listOrganizations({}).subscribe({ next: resolve, error: reject });
      }),
    ).rejects.toMatchObject({ code: Code.Internal, message: INTERNAL_MESSAGE });
  });

  it('leaves a call that went through alone', async () => {
    const transport = createRouterTransport(
      ({ service }) => {
        service(OrganizationService, {
          listOrganizations: () => create(ListOrganizationsResponseSchema, { organizations: [] }),
        });
      },
      { transport: { interceptors: [readableErrors] } },
    );
    const client = createObservableClient(OrganizationService, transport);

    await expect(
      new Promise((resolve, reject) => {
        client.listOrganizations({}).subscribe({ next: resolve, error: reject });
      }),
    ).resolves.toMatchObject({ organizations: [] });
  });

  it('rewrites a stream that fails after it started', async () => {
    const transport = createRouterTransport(
      ({ service }) => {
        service(LogsService, {
          // One entry first, so the failure comes while the stream is read
          // rather than when the call is made.
          async *tailLogs() {
            yield create(LogEntrySchema, {});
            throw new ConnectError('HTTP 503', Code.Unavailable);
          },
        });
      },
      { transport: { interceptors: [readableErrors] } },
    );
    const client = createObservableClient(LogsService, transport);

    await expect(
      new Promise((resolve, reject) => {
        client.tailLogs({}).subscribe({ complete: () => resolve(undefined), error: reject });
      }),
    ).rejects.toMatchObject({ code: Code.Unavailable, message: UNAVAILABLE_MESSAGE });
  });
});
