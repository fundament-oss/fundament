import { Given, When, Then } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { ICustomWorld } from '../support/world.ts';
import { APIKeyService, type APIKey } from '../support/api/apikey-service.ts';
import { type ExchangeTokenResponse } from '../support/api/token-service.ts';
import { ConnectRpcError } from '../support/api/client.ts';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { currentApiKey, API_TOKEN_PREFIX } from './common.steps.ts';

// Track state for token exchange tests
let savedToken: string | undefined;
let exchangeResponse: ExchangeTokenResponse | undefined;

// --- Given Steps ---

Given('I have saved the full token', async function (this: ICustomWorld) {
  savedToken = currentApiKey!.token;
});

Given('I have deleted the API key for exchange test', async function (this: ICustomWorld) {
  await this.apiKeyService!.deleteAPIKey(currentApiKey!.id);
  // Remove from cleanup map
  for (const [name, key] of this.createdApiKeys) {
    if (key.id === currentApiKey!.id) {
      this.createdApiKeys.delete(name);
      break;
    }
  }
});

// --- When Steps ---

When('I call ExchangeToken with the API token', async function (this: ICustomWorld) {
  try {
    exchangeResponse = await this.tokenService!.exchangeToken(savedToken!);
    this.lastApiResponse = exchangeResponse;
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
    exchangeResponse = undefined;
  }
});

When('I call ExchangeToken without an Authorization header', async function (this: ICustomWorld) {
  // Call without any token
  try {
    const response = await fetch(`${this.authnApiUrl}/authn.v1.TokenService/ExchangeToken`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Connect-Protocol-Version': '1',
      },
      body: JSON.stringify({}),
    });

    if (!response.ok) {
      const error = await response.json();
      throw new ConnectRpcError(error.code || 'unknown', error.message || 'Unknown error');
    }

    this.lastApiResponse = await response.json();
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
  }
});

When('I call ExchangeToken with token {string}', async function (this: ICustomWorld, token: string) {
  try {
    exchangeResponse = await this.tokenService!.exchangeToken(token);
    this.lastApiResponse = exchangeResponse;
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
    exchangeResponse = undefined;
  }
});

When('I call ExchangeToken with a valid-format but non-existent token', async function (this: ICustomWorld) {
  // Generate a token that has valid format but doesn't exist in DB
  // fun_ + 30 base62 chars + 6 checksum chars = 40 total
  // We'll use a known-good format with correct CRC but non-existent in DB
  const fakeToken = API_TOKEN_PREFIX + 'a'.repeat(30) + 'zzzzzz'; // Invalid checksum but valid characters

  try {
    exchangeResponse = await this.tokenService!.exchangeToken(fakeToken);
    this.lastApiResponse = exchangeResponse;
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
    exchangeResponse = undefined;
  }
});

When('I get the API key details', async function (this: ICustomWorld) {
  const response = await this.apiKeyService!.getAPIKey(currentApiKey!.id);
  this.lastApiResponse = response;
});

When('I use the exchanged JWT to list API keys', async function (this: ICustomWorld) {
  // Create a new APIKeyService with the exchanged JWT
  const exchangedService = new APIKeyService(this.organizationApiUrl!, exchangeResponse!.accessToken);
  try {
    const response = await exchangedService.listAPIKeys();
    this.lastApiResponse = response;
    this.lastApiError = undefined;
  } catch (error) {
    this.lastApiError = error as Error;
    this.lastApiResponse = undefined;
  }
});

// --- Then Steps ---

Then('I should receive a valid JWT', async function (this: ICustomWorld) {
  expect(exchangeResponse).toBeDefined();
  expect(exchangeResponse!.accessToken).toBeDefined();
  expect(exchangeResponse!.accessToken.length).toBeGreaterThan(0);
  // JWT has 3 parts separated by dots
  const parts = exchangeResponse!.accessToken.split('.');
  expect(parts.length).toBe(3);
});

Then('the JWT should expire in {int} seconds', async function (this: ICustomWorld, seconds: number) {
  expect(exchangeResponse).toBeDefined();
  expect(Number(exchangeResponse!.expiresIn)).toBe(seconds);
});

Then('the token type should be {string}', async function (this: ICustomWorld, tokenType: string) {
  expect(exchangeResponse).toBeDefined();
  expect(exchangeResponse!.tokenType).toBe(tokenType);
});

Then('the last used timestamp should be recent', async function (this: ICustomWorld) {
  const response = this.lastApiResponse as { apiKey: APIKey };
  expect(response.apiKey.lastUsed).toBeDefined();

  // Check that last_used is within the last minute
  const lastUsed = timestampDate(response.apiKey.lastUsed!);
  const now = new Date();
  const diffMs = Math.abs(now.getTime() - lastUsed.getTime());
  expect(diffMs).toBeLessThan(60000); // Within 1 minute
});

Then('the request should succeed', async function (this: ICustomWorld) {
  expect(this.lastApiError).toBeUndefined();
  expect(this.lastApiResponse).toBeDefined();
});
