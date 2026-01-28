import { Before, After, BeforeAll, AfterAll, Status, setDefaultTimeout } from '@cucumber/cucumber';
import { Browser, chromium } from 'playwright';
import { ICustomWorld } from './world.ts';
import { APIKeyService } from './api/apikey-service.ts';
import { TokenService } from './api/token-service.ts';
import * as dotenv from 'dotenv';

// Load environment variables
dotenv.config({ path: '.env.local' });

// Increase default step timeout for browser-based tests (default is 5000ms)
setDefaultTimeout(30000);

let browser: Browser;

BeforeAll(async function () {
  const isHeaded = process.env.HEADED === 'true';
  const launchOptions: Parameters<typeof chromium.launch>[0] = {
    headless: !isHeaded,
  };

  // Slow down actions in headed mode so we can see what's happening
  if (isHeaded) {
    launchOptions.slowMo = parseInt(process.env.SLOW_MO || '250', 10);
  }

  // Use executablePath for system browser (required on NixOS)
  // Falls back to channel for branded browsers like 'chrome' or 'msedge'
  if (process.env.BROWSER_PATH) {
    launchOptions.executablePath = process.env.BROWSER_PATH;
  } else if (process.env.BROWSER_CHANNEL) {
    launchOptions.channel = process.env.BROWSER_CHANNEL;
  }

  browser = await chromium.launch(launchOptions);
});

AfterAll(async function () {
  await browser?.close();
});

Before(async function (this: ICustomWorld) {
  this.browser = browser;
  this.context = await browser.newContext({
    baseURL: process.env.BASE_URL || 'http://console.127.0.0.1.nip.io:8080',
    viewport: { width: 1280, height: 720 },
  });
  this.page = await this.context.newPage();
});

After(async function (this: ICustomWorld, { result }) {
  // Capture screenshot on failure
  if (result?.status === Status.FAILED && this.page) {
    const screenshot = await this.page.screenshot();
    this.attach(screenshot, 'image/png');
  }

  await this.page?.close();
  await this.context?.close();
});

// API testing hooks for @api tagged scenarios
Before({ tags: '@api' }, async function (this: ICustomWorld) {
  this.organizationApiUrl = process.env.ORGANIZATION_API_URL || 'http://organization.127.0.0.1.nip.io:8080';
  this.authnApiUrl = process.env.AUTHN_API_URL || 'http://authn.127.0.0.1.nip.io:8080';
  this.tokenService = new TokenService(this.authnApiUrl);
  this.createdApiKeys = new Map();
  this.createdApiKeysByUser = new Map();
  this.savedApiKeyId = undefined;
  this.currentUserEmail = undefined;
  this.lastApiResponse = undefined;
  this.lastApiError = undefined;
});

After({ tags: '@api' }, async function (this: ICustomWorld) {
  // Cleanup: Delete all created API keys for each user
  for (const [userEmail, apiKeys] of this.createdApiKeysByUser) {
    if (apiKeys.size > 0) {
      try {
        // Authenticate as the user who created the keys
        const authToken = await authenticateForCleanup(this.authnApiUrl!, userEmail);
        const service = new APIKeyService(this.organizationApiUrl!, authToken);
        for (const [, apiKey] of apiKeys) {
          try {
            await service.deleteAPIKey(apiKey.id);
          } catch {
            // Ignore cleanup errors - key may already be deleted by test
          }
        }
      } catch {
        // Ignore auth errors during cleanup
      }
    }
  }

  // Also cleanup keys from the legacy map (for backwards compatibility)
  if (this.authToken && this.apiKeyService && this.createdApiKeys.size > 0) {
    for (const [, apiKey] of this.createdApiKeys) {
      try {
        await this.apiKeyService.deleteAPIKey(apiKey.id);
      } catch {
        // Ignore cleanup errors - key may already be deleted by test
      }
    }
  }
});

/**
 * Authenticate for cleanup purposes (separate from test flow).
 */
async function authenticateForCleanup(authnApiUrl: string, email: string): Promise<string> {
  const password = 'password';
  const response = await fetch(`${authnApiUrl}/login/password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  });
  if (!response.ok) {
    throw new Error(`Cleanup auth failed for ${email}`);
  }
  const data = (await response.json()) as { access_token: string };
  return data.access_token;
}
