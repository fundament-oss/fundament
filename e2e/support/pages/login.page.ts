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
    this.passwordInput = page.locator('nldd-password-field#password').locator('input');
    this.submitButton = page.locator('nldd-button[type="submit"]');
    this.errorMessage = page.locator('.text-danger-800, .text-danger-200');
    this.validationError = page.locator('nldd-form-field-error-text').filter({ hasText: /.+/ });
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
      return await this.errorMessage.textContent();
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
    const buttonText = await this.submitButton.textContent();
    return buttonText?.includes('Signing in') ?? false;
  }

  async isOnLoginPage(): Promise<boolean> {
    return this.page.url().includes('/login');
  }
}
