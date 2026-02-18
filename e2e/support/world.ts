import { World, IWorldOptions, setWorldConstructor } from '@cucumber/cucumber';
import { Browser, BrowserContext, Page } from 'playwright';
import { APIKeyService, type CreateAPIKeyResponse } from './api/apikey-service.ts';
import { TokenService } from './api/token-service.ts';

export interface ICustomWorld extends World {
  browser?: Browser;
  context?: BrowserContext;
  page?: Page;
  testData: Record<string, unknown>;
  // API testing state - base URLs
  organizationApiUrl?: string;
  authnApiUrl?: string;
  // Service clients
  apiKeyService?: APIKeyService;
  tokenService?: TokenService;
  authToken?: string;
  organizationId?: string;
  currentUserEmail?: string;
  // Track created API keys for cleanup (user email -> (name -> response))
  createdApiKeys: Map<string, CreateAPIKeyResponse>;
  createdApiKeysByUser: Map<string, Map<string, CreateAPIKeyResponse>>;
  // Saved API key ID for cross-user tests
  savedApiKeyId?: string;
  // Store the last API response for assertions
  lastApiResponse?: unknown;
  lastApiError?: Error;
}

export class CustomWorld extends World implements ICustomWorld {
  browser?: Browser;
  context?: BrowserContext;
  page?: Page;
  testData: Record<string, unknown> = {};
  // API testing state
  organizationApiUrl?: string;
  authnApiUrl?: string;
  apiKeyService?: APIKeyService;
  tokenService?: TokenService;
  authToken?: string;
  organizationId?: string;
  currentUserEmail?: string;
  createdApiKeys: Map<string, CreateAPIKeyResponse> = new Map();
  createdApiKeysByUser: Map<string, Map<string, CreateAPIKeyResponse>> = new Map();
  savedApiKeyId?: string;
  lastApiResponse?: unknown;
  lastApiError?: Error;

  constructor(options: IWorldOptions) {
    super(options);
  }
}

setWorldConstructor(CustomWorld);
