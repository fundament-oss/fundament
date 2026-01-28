import { Given, When, Then } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { ICustomWorld } from '../support/world.ts';
import { APIKeyService, type APIKey } from '../support/api/apikey-service.ts';
import { ConnectRpcError } from '../support/api/client.ts';
import { authenticateWithPassword, currentApiKey, setCurrentApiKey } from './common.steps.ts';

// --- Given Steps ---

Given('I save the API key ID', async function (this: ICustomWorld) {
  expect(currentApiKey).toBeDefined();
  this.savedApiKeyId = currentApiKey!.id;
});

// --- When Steps ---

When('I switch to user {string}', async function (this: ICustomWorld, email: string) {
  this.authToken = await authenticateWithPassword(email);
  this.currentUserEmail = email;
  this.apiKeyService = new APIKeyService(this.organizationApiUrl!, this.authToken);
  // Initialize the user's API key map if not exists
  if (!this.createdApiKeysByUser.has(email)) {
    this.createdApiKeysByUser.set(email, new Map());
  }
});

When('I try to get the saved API key by ID', async function (this: ICustomWorld) {
  try {
    const response = await this.apiKeyService!.getAPIKey(this.savedApiKeyId!);
    this.lastApiResponse = response;
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
  }
});

When('I try to revoke the saved API key', async function (this: ICustomWorld) {
  try {
    await this.apiKeyService!.revokeAPIKey(this.savedApiKeyId!);
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
  }
});

When('I try to delete the saved API key', async function (this: ICustomWorld) {
  try {
    await this.apiKeyService!.deleteAPIKey(this.savedApiKeyId!);
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
  }
});

// --- Then Steps ---

Then('I should NOT see the API key {string} in the list', async function (this: ICustomWorld, name: string) {
  const response = this.lastApiResponse as { apiKeys: APIKey[] };
  const found = (response.apiKeys || []).some((key) => key.name === name);
  expect(found).toBe(false);
});
