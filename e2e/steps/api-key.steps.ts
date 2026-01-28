import { Given, When, Then } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { ICustomWorld } from '../support/world.ts';
import { APIKeyService, type APIKey } from '../support/api/apikey-service.ts';
import { ConnectRpcError } from '../support/api/client.ts';
import { currentApiKey, setCurrentApiKey, API_TOKEN_PREFIX_DISPLAY_LENGTH } from './common.steps.ts';

// Track current API key details
let currentApiKeyDetails: APIKey | undefined;

// --- When Steps ---

When('I create an API key with name {string}', async function (this: ICustomWorld, name: string) {
  try {
    const response = await this.apiKeyService!.createAPIKey({ name });
    setCurrentApiKey(response);
    this.createdApiKeys.set(name, response);
    // Also track by user for cleanup
    if (this.currentUserEmail) {
      this.createdApiKeysByUser.get(this.currentUserEmail)?.set(name, response);
    }
    this.lastApiResponse = response;
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
  }
});

When('I create an API key with name {string} expiring in {int} days', async function (
  this: ICustomWorld,
  name: string,
  days: number
) {
  try {
    const response = await this.apiKeyService!.createAPIKey({ name, expiresInDays: days });
    setCurrentApiKey(response);
    this.createdApiKeys.set(name, response);
    // Also track by user for cleanup
    if (this.currentUserEmail) {
      this.createdApiKeysByUser.get(this.currentUserEmail)?.set(name, response);
    }
    this.lastApiResponse = response;
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
  }
});

When('I list all API keys', async function (this: ICustomWorld) {
  try {
    const response = await this.apiKeyService!.listAPIKeys();
    this.lastApiResponse = response;
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
  }
});

When('I get the API key by ID', async function (this: ICustomWorld) {
  try {
    const response = await this.apiKeyService!.getAPIKey(currentApiKey!.id);
    currentApiKeyDetails = response.apiKey;
    this.lastApiResponse = response;
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
  }
});

When('I revoke the API key', async function (this: ICustomWorld) {
  try {
    await this.apiKeyService!.revokeAPIKey(currentApiKey!.id);
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
  }
});

When('I delete the API key', async function (this: ICustomWorld) {
  try {
    await this.apiKeyService!.deleteAPIKey(currentApiKey!.id);
    // Remove from cleanup map since we deleted it
    for (const [name, key] of this.createdApiKeys) {
      if (key.id === currentApiKey!.id) {
        this.createdApiKeys.delete(name);
        break;
      }
    }
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
  }
});

When('I try to create another API key with name {string}', async function (this: ICustomWorld, name: string) {
  try {
    const response = await this.apiKeyService!.createAPIKey({ name });
    this.lastApiResponse = response;
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
  }
});

When('I try to create an API key with name {string}', async function (this: ICustomWorld, name: string) {
  // For unauthenticated scenario - create a service without auth
  const unauthService = new APIKeyService(this.organizationApiUrl!, '');
  try {
    const response = await unauthService.createAPIKey({ name });
    this.lastApiResponse = response;
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
  }
});

When('I try to revoke the API key again', async function (this: ICustomWorld) {
  try {
    await this.apiKeyService!.revokeAPIKey(currentApiKey!.id);
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
  }
});

When('I try to delete an API key with ID {string}', async function (this: ICustomWorld, id: string) {
  try {
    await this.apiKeyService!.deleteAPIKey(id);
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
  }
});

// --- Then Steps ---

Then('the response should contain a token starting with {string}', async function (
  this: ICustomWorld,
  prefix: string
) {
  expect(currentApiKey).toBeDefined();
  expect(currentApiKey!.token).toMatch(new RegExp(`^${prefix}`));
});

Then('the token should be {int} characters long', async function (this: ICustomWorld, length: number) {
  expect(currentApiKey).toBeDefined();
  expect(currentApiKey!.token.length).toBe(length);
});

Then('the response should contain a token prefix of {int} characters', async function (
  this: ICustomWorld,
  length: number
) {
  expect(currentApiKey).toBeDefined();
  expect(currentApiKey!.tokenPrefix.length).toBe(length);
});

Then('the API key should appear in the list of keys', async function (this: ICustomWorld) {
  const response = await this.apiKeyService!.listAPIKeys();
  const found = (response.apiKeys || []).some((key) => key.id === currentApiKey!.id);
  expect(found).toBe(true);
});

Then('the API key should have an expiration date', async function (this: ICustomWorld) {
  const response = await this.apiKeyService!.getAPIKey(currentApiKey!.id);
  expect(response.apiKey!.expiresAt).toBeDefined();
});

Then('the API key should be active', async function (this: ICustomWorld) {
  const response = await this.apiKeyService!.getAPIKey(currentApiKey!.id);
  expect(response.apiKey!.revokedAt).toBeUndefined();
});

Then('I should see the API key {string} in the list', async function (this: ICustomWorld, name: string) {
  const response = this.lastApiResponse as { apiKeys: APIKey[] };
  const found = (response.apiKeys || []).some((key) => key.name === name);
  expect(found).toBe(true);
});

Then('the API key should have a token prefix but not the full token', async function (this: ICustomWorld) {
  const response = this.lastApiResponse as { apiKeys: APIKey[] };
  for (const key of response.apiKeys || []) {
    expect(key.tokenPrefix).toBeDefined();
    expect(key.tokenPrefix.length).toBe(API_TOKEN_PREFIX_DISPLAY_LENGTH);
    // APIKey type doesn't have 'token' field - only tokenPrefix
    expect((key as unknown as { token?: string }).token).toBeUndefined();
  }
});

Then('I should see the key name {string}', async function (this: ICustomWorld, name: string) {
  expect(currentApiKeyDetails).toBeDefined();
  expect(currentApiKeyDetails!.name).toBe(name);
});

Then('I should see a created timestamp', async function (this: ICustomWorld) {
  expect(currentApiKeyDetails).toBeDefined();
  expect(currentApiKeyDetails!.createdAt).toBeDefined();
});

Then('I should NOT see the full token', async function (this: ICustomWorld) {
  // GetAPIKey response only has tokenPrefix, not full token
  expect((currentApiKeyDetails as unknown as { token?: string }).token).toBeUndefined();
});

Then('the API key should have a revoked timestamp', async function (this: ICustomWorld) {
  const response = await this.apiKeyService!.getAPIKey(currentApiKey!.id);
  expect(response.apiKey!.revokedAt).toBeDefined();
});

Then('the API key should still appear in the list', async function (this: ICustomWorld) {
  const response = await this.apiKeyService!.listAPIKeys();
  const found = (response.apiKeys || []).some((key) => key.id === currentApiKey!.id);
  expect(found).toBe(true);
});

Then('the API key should not appear in the list', async function (this: ICustomWorld) {
  const response = await this.apiKeyService!.listAPIKeys();
  const found = (response.apiKeys || []).some((key) => key.id === currentApiKey!.id);
  expect(found).toBe(false);
});

Then('getting the API key by ID should return not found', async function (this: ICustomWorld) {
  try {
    await this.apiKeyService!.getAPIKey(currentApiKey!.id);
    throw new Error('Expected not found error');
  } catch (error) {
    expect(error).toBeInstanceOf(ConnectRpcError);
    expect((error as ConnectRpcError).code).toBe('not_found');
  }
});
