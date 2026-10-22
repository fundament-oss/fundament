import { World, IWorldOptions, setWorldConstructor } from '@cucumber/cucumber';
import { Browser, BrowserContext, Page } from 'playwright';
import {
  APIKeyService,
  type CreateAPIKeyResponse,
} from './api/apikey-service.ts';
import { TokenService } from './api/token-service.ts';
import { ProjectService } from './api/project-service.ts';

export interface CreatedProject {
  id: string;
  ownerEmail: string;
}
import { ClusterService, type ClusterDetails } from './api/cluster-service.ts';

export interface ICustomWorld extends World {
  browser?: Browser;
  context?: BrowserContext;
  page?: Page;
  testData: Record<string, unknown>;
  // API testing state - base URLs
  organizationApiUrl?: string;
  authnApiUrl?: string;
  pluginProxyUrl?: string;
  consoleUrl?: string;
  // Service clients
  apiKeyService?: APIKeyService;
  tokenService?: TokenService;
  projectService?: ProjectService;
  authToken?: string;
  organizationId?: string;
  currentUserEmail?: string;
  // Track created API keys for cleanup (user email -> (name -> response))
  createdApiKeys: Map<string, CreateAPIKeyResponse>;
  createdApiKeysByUser: Map<string, Map<string, CreateAPIKeyResponse>>;
  // Saved API key ID for cross-user tests
  savedApiKeyId?: string;
  // Project created by the current scenario, and all of them (with the
  // user who created them) for cleanup
  currentProjectId?: string;
  createdProjects: CreatedProject[];
  // Store the last API response for assertions
  lastApiResponse?: unknown;
  lastApiError?: Error;
  // @real-gardener node pool provisioning state
  clusterService?: ClusterService;
  realGardenerCluster?: ClusterDetails;
}

export class CustomWorld extends World implements ICustomWorld {
  browser?: Browser;
  context?: BrowserContext;
  page?: Page;
  testData: Record<string, unknown> = {};
  // API testing state
  organizationApiUrl?: string;
  authnApiUrl?: string;
  pluginProxyUrl?: string;
  consoleUrl?: string;
  apiKeyService?: APIKeyService;
  tokenService?: TokenService;
  projectService?: ProjectService;
  authToken?: string;
  organizationId?: string;
  currentUserEmail?: string;
  createdApiKeys: Map<string, CreateAPIKeyResponse> = new Map();
  createdApiKeysByUser: Map<string, Map<string, CreateAPIKeyResponse>> =
    new Map();
  currentProjectId?: string;
  createdProjects: CreatedProject[] = [];
  savedApiKeyId?: string;
  lastApiResponse?: unknown;
  lastApiError?: Error;
  clusterService?: ClusterService;
  realGardenerCluster?: ClusterDetails;

  constructor(options: IWorldOptions) {
    super(options);
  }
}

setWorldConstructor(CustomWorld);
