import {
  Before,
  After,
  BeforeAll,
  AfterAll,
  Status,
  setDefaultTimeout,
} from '@cucumber/cucumber';
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

// Playwright traces record cookies and page contents, so they are opt-in:
// E2E_TRACE=on (the test-env VM sets it).
const tracing = process.env.E2E_TRACE === 'on';

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
    baseURL: process.env.BASE_URL || 'https://console.fundament.localhost:8443',
    viewport: { width: 1280, height: 720 },
  });
  if (tracing) {
    await this.context.tracing.start({ screenshots: true, snapshots: true });
  }
  this.page = await this.context.newPage();
});

After(async function (this: ICustomWorld, { result, pickle }) {
  const failed = result?.status === Status.FAILED;
  if (failed && this.page) {
    const screenshot = await this.page.screenshot();
    this.attach(screenshot, 'image/png');
  }

  try {
    // A Playwright trace per failed scenario: `bunx playwright show-trace <zip>`.
    if (tracing && this.context) {
      if (failed) {
        const slug = pickle.name.toLowerCase().replace(/[^a-z0-9]+/g, '-').slice(0, 60);
        const path = `reports/traces/${slug}-${pickle.id.slice(0, 8)}.zip`;
        await this.context.tracing.stop({ path });
        this.attach(`trace: e2e/${path}`, 'text/plain');
      } else {
        await this.context.tracing.stop();
      }
    }
  } finally {
    await this.page?.close();
    await this.context?.close();
  }
});

// @real-gardener scenarios create shoots, node pools and plugin installations,
// which only a real Gardener does (E2E_GARDENER=real, the test-env VM). The
// default profile leaves them out; under the full profile they skip elsewhere,
// so they show in the report rather than vanish.
Before({ tags: '@real-gardener' }, function () {
  return process.env.E2E_GARDENER === 'real' ? undefined : 'skipped';
});

// @mock-gardener scenarios rely on clusters that exist only in the database,
// so they skip against a real Gardener.
Before({ tags: '@mock-gardener' }, function () {
  return process.env.E2E_GARDENER === 'real' ? 'skipped' : undefined;
});

// @shoot-token scenarios need ServiceAccount tokens authn-api accepts: real
// ones from a shoot (E2E_GARDENER=real), or HMAC stand-ins for an authn-api in
// SHOOT_VERIFIER_MODE=mock (PR previews). Against any other deployment they
// would fail for the wrong reason, so they skip.
Before({ tags: '@shoot-token' }, function () {
  if (process.env.E2E_GARDENER !== 'real' && process.env.E2E_MOCK_SHOOT_VERIFIER !== 'true') {
    return 'skipped';
  }
  return undefined;
});

// API testing hooks for @api tagged scenarios
Before({ tags: '@api' }, async function (this: ICustomWorld) {
  this.organizationApiUrl =
    process.env.ORGANIZATION_API_URL ||
    'https://organization.fundament.localhost:8443';
  this.authnApiUrl =
    process.env.AUTHN_API_URL || 'https://authn.fundament.localhost:8443';
  this.pluginProxyUrl =
    process.env.PLUGIN_PROXY_URL ||
    'https://plugin-proxy.fundament.localhost:8443';
  this.consoleUrl =
    process.env.BASE_URL || 'https://console.fundament.localhost:8443';
  this.tokenService = new TokenService(this.authnApiUrl);
  this.createdApiKeys = new Map();
  this.createdApiKeysByUser = new Map();
  this.savedApiKeyId = undefined;
  this.organizationId = undefined;
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
        const { token, organizationId } = await authenticateForCleanup(
          this.authnApiUrl!,
          userEmail,
        );
        const service = new APIKeyService(
          this.organizationApiUrl!,
          token,
          organizationId,
        );
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
 * Returns the auth token and the first organization ID from the JWT.
 */
async function authenticateForCleanup(
  authnApiUrl: string,
  email: string,
): Promise<{ token: string; organizationId: string }> {
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
  const token = data.access_token;
  const payload = JSON.parse(
    Buffer.from(token.split('.')[1], 'base64url').toString(),
  );
  const orgIds: string[] = payload.organization_ids ?? [];
  return { token, organizationId: orgIds[0] };
}
