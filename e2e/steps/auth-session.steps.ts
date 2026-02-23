import { Given, When, Then } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { ICustomWorld } from '../support/world.ts';
import { LoginPage } from '../support/pages/login.page.ts';

const AUTH_COOKIE_NAME = 'fundament_auth';

// JWT utility functions for tampering tests
function base64UrlDecode(str: string): string {
  // Add padding if needed
  const padding = '='.repeat((4 - (str.length % 4)) % 4);
  const base64 = str.replace(/-/g, '+').replace(/_/g, '/') + padding;
  return Buffer.from(base64, 'base64').toString('utf-8');
}

function base64UrlEncode(str: string): string {
  return Buffer.from(str, 'utf-8')
    .toString('base64')
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '');
}


Given(
  'I am logged in as {string} with password {string}',
  async function (this: ICustomWorld, email: string, password: string) {
    const loginPage = new LoginPage(this.page!);
    await loginPage.goto();
    await loginPage.login(email, password);
    await loginPage.waitForLoginSuccess();
  }
);

Given('my session is active', async function (this: ICustomWorld) {
  // Verify we have an auth cookie
  const cookies = await this.context!.cookies();
  const authCookie = cookies.find((c) => c.name === AUTH_COOKIE_NAME);
  expect(authCookie).toBeDefined();
});

When('I navigate to the dashboard', async function (this: ICustomWorld) {
  await this.page!.goto('/');
  await this.page!.waitForLoadState('networkidle');
});

When(
  'I navigate to a page that loads organization data',
  async function (this: ICustomWorld) {
    await this.page!.goto('/organization');
    await this.page!.waitForLoadState('networkidle');
  }
);

When('I trigger a token refresh', async function (this: ICustomWorld) {
  const authnApiUrl =
    process.env.AUTHN_API_URL || 'http://authn.fundament.localhost:8080';

  // Trigger refresh by calling the refresh endpoint via the page context
  const response = await this.page!.evaluate(async (url: string) => {
    const res = await fetch(`${url}/refresh`, {
      method: 'POST',
      credentials: 'include',
    });
    return { ok: res.ok, status: res.status };
  }, authnApiUrl);
  this.testData.refreshResponse = response;
});

When('I click the logout button', async function (this: ICustomWorld) {
  // Open user dropdown
  const userMenuButton = this.page!.locator('.user-dropdown button').first();
  await userMenuButton.click();

  // Click logout
  const logoutButton = this.page!.getByRole('button', { name: 'Log out' });
  await logoutButton.click();

  // Wait for navigation
  await this.page!.waitForLoadState('networkidle');
});

Then('I should see the dashboard content', async function (this: ICustomWorld) {
  // Dashboard has a heading with "Dashboard"
  const heading = this.page!.locator('h1:has-text("Dashboard")');
  await expect(heading).toBeVisible({ timeout: 10000 });
});

Then('the auth cookie should be set', async function (this: ICustomWorld) {
  const cookies = await this.context!.cookies();
  const authCookie = cookies.find((c) => c.name === AUTH_COOKIE_NAME);

  expect(authCookie).toBeDefined();
  expect(authCookie!.httpOnly).toBe(true);
  expect(authCookie!.sameSite).toBe('Strict');
});

Then(
  'the organization data should load successfully',
  async function (this: ICustomWorld) {
    // Wait for loading to finish and data to appear
    await this.page!.waitForSelector('text=Loading organization', {
      state: 'hidden',
      timeout: 10000,
    });

    // Check that organization ID is displayed (indicates successful API call)
    const orgIdLabel = this.page!.locator('text=Organization ID');
    await expect(orgIdLabel).toBeVisible();

    // Check that the actual ID value is shown (a UUID-like string in a mono font element)
    const orgIdValue = this.page!.locator('.font-mono');
    await expect(orgIdValue).toBeVisible();
    const idText = await orgIdValue.textContent();
    expect(idText?.trim()).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
    );
  }
);

Then(
  'I should not see an authentication error',
  async function (this: ICustomWorld) {
    // Check that no error message is visible
    const errorMessage = this.page!.locator('.bg-red-50, .bg-red-950');
    await expect(errorMessage).not.toBeVisible();
  }
);

Then('I should remain authenticated', async function (this: ICustomWorld) {
  const response = this.testData.refreshResponse as {
    ok: boolean;
    status: number;
  };
  expect(response.ok).toBe(true);
});

Then(
  'I should still have access to the dashboard',
  async function (this: ICustomWorld) {
    await this.page!.goto('/');
    await this.page!.waitForLoadState('networkidle');

    // Should not be redirected to login
    expect(this.page!.url()).not.toContain('/login');

    // Dashboard content should be visible
    const heading = this.page!.locator('h1:has-text("Dashboard")');
    await expect(heading).toBeVisible({ timeout: 10000 });
  }
);

Then(
  'I should be redirected to the login page',
  async function (this: ICustomWorld) {
    await this.page!.waitForURL('**/login', { timeout: 10000 });
    expect(this.page!.url()).toContain('/login');
  }
);

Then('the auth cookie should be cleared', async function (this: ICustomWorld) {
  const cookies = await this.context!.cookies();
  const authCookie = cookies.find((c) => c.name === AUTH_COOKIE_NAME);

  // Cookie should either not exist or be empty/expired
  if (authCookie) {
    expect(authCookie.value).toBe('');
  }
});

Then(
  'I should not be able to access the dashboard directly',
  async function (this: ICustomWorld) {
    // Try to navigate to dashboard
    await this.page!.goto('/');
    await this.page!.waitForLoadState('networkidle');

    // Should be redirected to login
    await this.page!.waitForURL('**/login', { timeout: 10000 });
    expect(this.page!.url()).toContain('/login');
  }
);

// JWT Tampering Test Steps

Given('I have a valid auth cookie', async function (this: ICustomWorld) {
  const cookies = await this.context!.cookies();
  const authCookie = cookies.find((c) => c.name === AUTH_COOKIE_NAME);
  expect(authCookie).toBeDefined();
  expect(authCookie!.value).toBeTruthy();

  // Store the original token for tampering
  this.testData.originalToken = authCookie!.value;
  this.testData.cookieDomain = authCookie!.domain;
  this.testData.cookiePath = authCookie!.path;
});

When(
  'I modify the JWT payload to change the organization ID',
  async function (this: ICustomWorld) {
    const token = this.testData.originalToken as string;
    const parts = token.split('.');

    // Decode and modify payload
    const payload = JSON.parse(base64UrlDecode(parts[1]));
    // Change org ID to a fake UUID
    payload.OrganizationID = '00000000-0000-0000-0000-000000000000';

    // Re-encode with original signature (which won't match)
    const tamperedToken = `${parts[0]}.${base64UrlEncode(JSON.stringify(payload))}.${parts[2]}`;
    this.testData.tamperedToken = tamperedToken;
  }
);

When('I corrupt the JWT signature', async function (this: ICustomWorld) {
  const token = this.testData.originalToken as string;
  const parts = token.split('.');

  // Corrupt the signature by changing some characters
  const corruptedSignature = parts[2].split('').reverse().join('');

  const tamperedToken = `${parts[0]}.${parts[1]}.${corruptedSignature}`;
  this.testData.tamperedToken = tamperedToken;
});

When(
  'I modify the JWT to use the none algorithm',
  async function (this: ICustomWorld) {
    const token = this.testData.originalToken as string;
    const parts = token.split('.');

    // Create header with "none" algorithm
    const noneHeader = { alg: 'none', typ: 'JWT' };

    // Create token with no signature
    const tamperedToken = `${base64UrlEncode(JSON.stringify(noneHeader))}.${parts[1]}.`;
    this.testData.tamperedToken = tamperedToken;
  }
);

When('I remove the JWT signature', async function (this: ICustomWorld) {
  const token = this.testData.originalToken as string;
  const parts = token.split('.');

  // Remove signature but keep the structure
  const tamperedToken = `${parts[0]}.${parts[1]}.`;
  this.testData.tamperedToken = tamperedToken;
});

When(
  'I set the auth cookie to a random invalid value',
  async function (this: ICustomWorld) {
    this.testData.tamperedToken = 'not-a-valid-jwt-token';
    this.testData.cookieDomain = 'fundament.localhost';
    this.testData.cookiePath = '/';
  }
);

When(
  'I make an API request with the tampered token',
  async function (this: ICustomWorld) {
    const tamperedToken = this.testData.tamperedToken as string;
    const domain = (this.testData.cookieDomain as string) || 'fundament.localhost';
    const path = (this.testData.cookiePath as string) || '/';

    // Clear existing cookies and set the tampered one
    await this.context!.clearCookies();
    await this.context!.addCookies([
      {
        name: AUTH_COOKIE_NAME,
        value: tamperedToken,
        domain: domain,
        path: path,
      },
    ]);

    // Make API request via the organization API
    const orgApiUrl =
      process.env.ORGANIZATION_API_URL || 'http://organization.fundament.localhost:8080';

    const response = await this.page!.evaluate(async (url: string) => {
      try {
        const res = await fetch(`${url}/organization.v1.OrganizationService/GetOrganization`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          credentials: 'include',
          body: JSON.stringify({}),
        });
        return { status: res.status, ok: res.ok };
      } catch (e) {
        return { status: 0, ok: false, error: String(e) };
      }
    }, orgApiUrl);

    this.testData.apiResponse = response;
  }
);

Then(
  'the API request should be rejected with an authentication error',
  async function (this: ICustomWorld) {
    const response = this.testData.apiResponse as { status: number; ok: boolean };

    // The request should fail with 401 Unauthorized
    expect(response.ok).toBe(false);
    expect(response.status).toBe(401);
  }
);
