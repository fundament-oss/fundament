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

When('I enter email {string}', async function (this: ICustomWorld, email: string) {
  await loginPage.emailInput.fill(email);
});

When('I enter password {string}', async function (this: ICustomWorld, password: string) {
  await loginPage.passwordInput.fill(password);
});

When('I click the sign in button', async function (this: ICustomWorld) {
  await loginPage.submitButton.click();
});

Then('I should be redirected to the dashboard', async function (this: ICustomWorld) {
  await loginPage.waitForLoginSuccess();
  // Dashboard is at root path
  await expect(this.page!).toHaveURL('/');
});

Then('I should see the main navigation', async function (this: ICustomWorld) {
  // Verify we're on the dashboard by checking for main content
  const mainContent = this.page!.locator('main');
  await expect(mainContent).toBeVisible();
});

Then('I should see an error message', async function (this: ICustomWorld) {
  await expect(loginPage.errorMessage).toBeVisible({ timeout: 5000 });
});

Then('I should see an error message {string}', async function (this: ICustomWorld, expectedMessage: string) {
  await expect(loginPage.errorMessage).toBeVisible({ timeout: 5000 });
  await expect(loginPage.errorMessage).toContainText(expectedMessage);
});

Then('I should see a validation error containing {string}', async function (this: ICustomWorld, errorText: string) {
  await expect(loginPage.validationError).toBeVisible({ timeout: 3000 });
  await expect(loginPage.validationError).toContainText(errorText);
});

Then('I should remain on the login page', async function (this: ICustomWorld) {
  const isOnLogin = await loginPage.isOnLoginPage();
  expect(isOnLogin).toBe(true);
});
