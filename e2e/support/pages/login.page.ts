import { Page, Locator } from 'playwright';

export class LoginPage {
  readonly page: Page;
  readonly emailInput: Locator;
  readonly passwordInput: Locator;
  readonly submitButton: Locator;
  readonly errorMessage: Locator;
  readonly validationError: Locator;
  readonly heading: Locator;

  constructor(page: Page) {
    this.page = page;
    this.emailInput = page.locator('nldd-text-field#email').locator('input');
    this.passwordInput = page
      .locator('nldd-password-field#password')
      .locator('input');
    this.submitButton = page.locator('nldd-button[type="submit"]');
    // The form reports a failed sign-in as a critical banner above it.
    this.errorMessage = page.locator('nldd-banner[variant="critical"]');
    this.validationError = page
      .locator('nldd-validation-item')
      .filter({ hasText: /\S/ });
    this.heading = page.getByRole('heading', { name: 'Log in' });
  }

  async goto() {
    await this.page.goto('/login');
    await this.page.waitForLoadState('networkidle');
  }

  async login(email: string, password: string) {
    await this.emailInput.fill(email);
    await this.passwordInput.fill(password);
    await this.submitButton.click();
  }

  async waitForLoginSuccess() {
    // Wait for redirect away from login page
    await this.page.waitForURL((url) => !url.pathname.includes('/login'), {
      timeout: 10000,
    });
  }

  async getErrorMessage(): Promise<string | null> {
    try {
      await this.errorMessage.waitFor({ state: 'visible', timeout: 5000 });
      // The banner keeps its wording in attributes, not in light DOM text.
      const [text, supporting] = await Promise.all([
        this.errorMessage.getAttribute('text'),
        this.errorMessage.getAttribute('supporting-text'),
      ]);
      return [text, supporting].filter(Boolean).join(' ');
    } catch {
      return null;
    }
  }

  async getValidationError(): Promise<string | null> {
    try {
      await this.validationError.waitFor({ state: 'visible', timeout: 3000 });
      return await this.validationError.textContent();
    } catch {
      return null;
    }
  }

  async isLoading(): Promise<boolean> {
    // The button says it is busy with an attribute rather than with its label.
    return (await this.submitButton.getAttribute('loading')) !== null;
  }

  async isOnLoginPage(): Promise<boolean> {
    return this.page.url().includes('/login');
  }
}
