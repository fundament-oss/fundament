import { Given, When, Then } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { ICustomWorld } from '../support/world.ts';
import {
  APIKeyService,
  type CreateAPIKeyResponse,
} from '../support/api/apikey-service.ts';
import { ConnectRpcError } from '../support/api/client.ts';
import {
  authenticateWithPassword,
  extractOrganizationId,
  waitForAuthzReady,
  AUTHZ_READY_TIMEOUT_MS,
} from '../support/auth.ts';

// API token format constants (must match Go apitoken package)
export const API_TOKEN_PREFIX = 'fun_';
export const API_TOKEN_TOTAL_LENGTH = 40;
export const API_TOKEN_PREFIX_DISPLAY_LENGTH = 8;

// Shared state for current API key being worked with
export let currentApiKey: CreateAPIKeyResponse | undefined;

export function setCurrentApiKey(key: CreateAPIKeyResponse | undefined) {
  currentApiKey = key;
}

// --- Common Given Steps ---

Given(
  'I am authenticated as {string}',
  { timeout: AUTHZ_READY_TIMEOUT_MS + 10_000 },
  async function (this: ICustomWorld, email: string) {
    this.authToken = await authenticateWithPassword(email);
    this.organizationId = extractOrganizationId(this.authToken);
    this.currentUserEmail = email;
    this.apiKeyService = new APIKeyService(
      this.organizationApiUrl!,
      this.authToken,
      this.organizationId,
    );
    // Initialize the user's API key map if not exists
    if (!this.createdApiKeysByUser.has(email)) {
      this.createdApiKeysByUser.set(email, new Map());
    }
    // Tolerate authz-worker -> OpenFGA propagation lag before any
    // permission-gated step runs (otherwise CreateAPIKey etc. flake with
    // permission_denied right after a fresh deploy).
    const service = this.apiKeyService;
    await waitForAuthzReady(() => service.listAPIKeys());
  },
);

Given('I have no authentication', async function (this: ICustomWorld) {
  this.authToken = undefined;
  this.apiKeyService = undefined;
});

Given(
  'I have created an API key named {string}',
  async function (this: ICustomWorld, name: string) {
    let response;
    try {
      response = await this.apiKeyService!.createAPIKey({ name, expiresIn: '1h' });
    } catch (err) {
      if (err instanceof ConnectRpcError && err.code === 'already_exists') {
        // A leftover key from a previous test run. Delete it and retry.
        const list = await this.apiKeyService!.listAPIKeys();
        const existing = list.apiKeys.find((k) => k.name === name);
        if (existing) {
          await this.apiKeyService!.deleteAPIKey(existing.id);
        }
        response = await this.apiKeyService!.createAPIKey({ name, expiresIn: '1h' });
      } else {
        throw err;
      }
    }
    currentApiKey = response;

    this.createdApiKeys.set(name, response);
    // Also track by user for cleanup
    if (this.currentUserEmail) {
      this.createdApiKeysByUser.get(this.currentUserEmail)?.set(name, response);
    }
  },
);

Given(
  'I have created an API key named {string} with expiry {string}',
  async function (this: ICustomWorld, name: string, expiresIn: string) {
    let response;
    try {
      response = await this.apiKeyService!.createAPIKey({ name, expiresIn });
    } catch (err) {
      if (err instanceof ConnectRpcError && err.code === 'already_exists') {
        const list = await this.apiKeyService!.listAPIKeys();
        const existing = list.apiKeys.find((k) => k.name === name);
        if (existing) {
          await this.apiKeyService!.deleteAPIKey(existing.id);
        }
        response = await this.apiKeyService!.createAPIKey({ name, expiresIn });
      } else {
        throw err;
      }
    }
    currentApiKey = response;

    this.createdApiKeys.set(name, response);
    if (this.currentUserEmail) {
      this.createdApiKeysByUser.get(this.currentUserEmail)?.set(name, response);
    }
  },
);

Given('I have revoked the API key', async function (this: ICustomWorld) {
  await this.apiKeyService!.revokeAPIKey(currentApiKey!.id);
});

Given(
  'I wait {int} seconds',
  async function (this: ICustomWorld, seconds: number) {
    await new Promise((resolve) => setTimeout(resolve, seconds * 1000));
  },
);

// --- Common When Steps ---

When('I delete the API key', async function (this: ICustomWorld) {
  try {
    await this.apiKeyService!.deleteAPIKey(currentApiKey!.id);
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
  }
});

When(
  'I try to create an API key named {string} with expiry {string}',
  async function (this: ICustomWorld, name: string, expiresIn: string) {
    try {
      const response = await this.apiKeyService!.createAPIKey({
        name,
        expiresIn,
      });
      // Tracked so the cleanup removes a key that should have been refused.
      this.createdApiKeys.set(name, response);
      if (this.currentUserEmail) {
        this.createdApiKeysByUser
          .get(this.currentUserEmail)
          ?.set(name, response);
      }
      this.lastApiError = undefined;
    } catch (error) {
      this.lastApiError = error as Error;
    }
  },
);

// --- Common Then Steps ---

Then('I should not receive an error', async function (this: ICustomWorld) {
  expect(this.lastApiError).toBeUndefined();
});

Then('I should receive an error', async function (this: ICustomWorld) {
  expect(this.lastApiError).toBeDefined();
});

Then(
  'I should receive an unauthenticated error',
  async function (this: ICustomWorld) {
    expect(this.lastApiError).toBeDefined();
    expect(this.lastApiError).toBeInstanceOf(ConnectRpcError);
    expect((this.lastApiError as ConnectRpcError).code).toBe('unauthenticated');
  },
);

Then('I should receive a not found error', async function (this: ICustomWorld) {
  expect(this.lastApiError).toBeDefined();
  expect(this.lastApiError).toBeInstanceOf(ConnectRpcError);
  expect((this.lastApiError as ConnectRpcError).code).toBe('not_found');
});

Then(
  'the error message should contain {string}',
  async function (this: ICustomWorld, expected: string) {
    expect(this.lastApiError).toBeDefined();
    expect(this.lastApiError!.message).toContain(expected);
  },
);
