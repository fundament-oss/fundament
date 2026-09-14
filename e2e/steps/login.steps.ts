import { Given, When, Then } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { ICustomWorld } from '../support/world.ts';
import { LoginPage } from '../support/pages/login.page.ts';

let loginPage: LoginPage;

Given('I am on the login page', async function (this: ICustomWorld) {
  loginPage = new LoginPage(this.page!);
  await loginPage.goto();
  await expect(loginPage.heading).toBeVisible();
});

When(
  'I enter email {string}',
  async function (this: ICustomWorld, email: string) {
    await loginPage.emailInput.fill(email);
  },
);

When(
  'I enter password {string}',
  async function (this: ICustomWorld, password: string) {
    await loginPage.passwordInput.fill(password);
  },
);

When('I click the sign in button', async function (this: ICustomWorld) {
  await loginPage.submitButton.click();
});

Then(
  'I should be redirected to the dashboard',
  async function (this: ICustomWorld) {
    await loginPage.waitForLoginSuccess();
    // Logging in lands on the clusters, inside the organization the address names.
    await expect(this.page!).toHaveURL(/\/organizations\/[^/]+\/clusters$/, {
      timeout: 10000,
    });
  },
);

Then('I should see the main navigation', async function (this: ICustomWorld) {
  // The shell's toolbar is what only a logged-in page has.
  const userMenu = this.page!.locator(
    'nldd-icon-button[accessible-label="User menu"]',
  );
  await expect(userMenu).toBeVisible({ timeout: 10000 });
});

Then('I should see an error message', async function (this: ICustomWorld) {
  await expect(loginPage.errorMessage).toBeVisible({ timeout: 5000 });
});

Then(
  'I should see an error message {string}',
  async function (this: ICustomWorld, expectedMessage: string) {
    await expect(loginPage.errorMessage).toBeVisible({ timeout: 5000 });
    expect(await loginPage.getErrorMessage()).toContain(expectedMessage);
  },
);

Then(
  'I should see a validation error containing {string}',
  async function (this: ICustomWorld, errorText: string) {
    await expect(loginPage.validationError).toBeVisible({ timeout: 3000 });
    await expect(loginPage.validationError).toContainText(errorText);
  },
);

Then('I should remain on the login page', async function (this: ICustomWorld) {
  const isOnLogin = await loginPage.isOnLoginPage();
  expect(isOnLogin).toBe(true);
});
