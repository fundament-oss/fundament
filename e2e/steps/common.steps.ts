import { Given, Then } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { ICustomWorld } from '../support/world.ts';
import { APIKeyService, type CreateAPIKeyResponse } from '../support/api/apikey-service.ts';
import { ConnectRpcError } from '../support/api/client.ts';

// API token format constants (must match Go apitoken package)
export const API_TOKEN_PREFIX = 'fun_';
export const API_TOKEN_TOTAL_LENGTH = 40;
export const API_TOKEN_PREFIX_DISPLAY_LENGTH = 8;

// Shared state for current API key being worked with
export let currentApiKey: CreateAPIKeyResponse | undefined;

export function setCurrentApiKey(key: CreateAPIKeyResponse | undefined) {
  currentApiKey = key;
}

/**
 * Authenticate via password login and get JWT.
 */
export async function authenticateWithPassword(email: string): Promise<string> {
  const authnApiUrl = process.env.AUTHN_API_URL || 'http://authn.127.0.0.1.nip.io:8080';
  const password = 'password';

  const response = await fetch(`${authnApiUrl}/login/password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  });

  if (!response.ok) {
    const error = await response.text();
    throw new Error(`Authentication failed: ${error}`);
  }

  const data = (await response.json()) as { access_token: string };
  if (!data.access_token) {
    throw new Error('No access_token in login response');
  }

  return data.access_token;
}

// --- Common Given Steps ---

Given('I am authenticated as {string}', async function (this: ICustomWorld, email: string) {
  this.authToken = await authenticateWithPassword(email);
  this.currentUserEmail = email;
  this.apiKeyService = new APIKeyService(this.organizationApiUrl!, this.authToken);
  // Initialize the user's API key map if not exists
  if (!this.createdApiKeysByUser.has(email)) {
    this.createdApiKeysByUser.set(email, new Map());
  }
});

Given('I have no authentication', async function (this: ICustomWorld) {
  this.authToken = undefined;
  this.apiKeyService = undefined;
});

Given('I have created an API key named {string}', async function (this: ICustomWorld, name: string) {
  const response = await this.apiKeyService!.createAPIKey({ name });
  currentApiKey = response;
  this.createdApiKeys.set(name, response);
  // Also track by user for cleanup
  if (this.currentUserEmail) {
    this.createdApiKeysByUser.get(this.currentUserEmail)?.set(name, response);
  }
});

Given('I have revoked the API key', async function (this: ICustomWorld) {
  await this.apiKeyService!.revokeAPIKey(currentApiKey!.id);
});

// --- Common Then Steps ---

Then('I should receive an error', async function (this: ICustomWorld) {
  expect(this.lastApiError).toBeDefined();
});

Then('I should receive an unauthenticated error', async function (this: ICustomWorld) {
  expect(this.lastApiError).toBeDefined();
  expect(this.lastApiError).toBeInstanceOf(ConnectRpcError);
  expect((this.lastApiError as ConnectRpcError).code).toBe('unauthenticated');
});

Then('I should receive a not found error', async function (this: ICustomWorld) {
  expect(this.lastApiError).toBeDefined();
  expect(this.lastApiError).toBeInstanceOf(ConnectRpcError);
  expect((this.lastApiError as ConnectRpcError).code).toBe('not_found');
});
