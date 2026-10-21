import { Page, Locator } from 'playwright';
import { expect } from '@playwright/test';

/** The sheet that edits a cluster's node pools (Node pools → Edit). */
export class NodePoolsPage {
  readonly page: Page;
  readonly newNodePoolButton: Locator;
  readonly saveButton: Locator;
  readonly errorBanner: Locator;

  constructor(page: Page) {
    this.page = page;
    this.newNodePoolButton = page.locator('nldd-button[text="New node pool"]');
    this.saveButton = page.locator('nldd-button[text="Save changes"]');
    this.errorBanner = page.locator('nldd-banner[variant="critical"]');
  }

  async goto(organizationName: string, clusterId: string) {
    await this.page.goto(
      `/organizations/${organizationName}/clusters/${clusterId}/nodes`,
    );
    // The bounds come from the quotas, which load next to the pools and the
    // catalog: until the hint is there, the form does not know them yet.
    await expect(this.autoscaleMaxHint(0)).toContainText('this pool', {
      timeout: 15000,
    });
  }

  /** The box of the pool at `index`, as the form numbers them from 1. */
  pool(index: number): Locator {
    return this.page.locator('nldd-box').filter({
      has: this.page.getByRole('heading', { name: `Node pool ${index + 1}` }),
    });
  }

  async chooseMachineType(index: number, machineType: string) {
    // The row's label starts with the name and goes on with the specs.
    await this.pool(index)
      .locator('nldd-list-item')
      .filter({
        has: this.page.locator(`nldd-text-cell[text^="${machineType} "]`),
      })
      .click();
  }

  async setName(index: number, name: string) {
    await this.page
      .locator(`#nodePoolName-${index}`)
      .locator('input')
      .fill(name);
  }

  async setAutoscaleMax(index: number, value: number) {
    const input = this.page.locator(`#autoscaleMax-${index}`).locator('input');
    await input.fill(String(value));
    await input.press('Tab');
  }

  autoscaleMaxHint(index: number): Locator {
    return this.pool(index)
      .locator('nldd-form-field[label="Autoscale maximum"]')
      .locator('nldd-form-field-help-text');
  }

  autoscaleMaxError(index: number): Locator {
    return this.page.locator(`#autoscale-max-error-${index}`);
  }

  async errorText(): Promise<string> {
    await this.errorBanner.waitFor({ state: 'visible', timeout: 10000 });
    // The banner keeps its wording in an attribute, not in light DOM text.
    return (await this.errorBanner.getAttribute('text')) ?? '';
  }
}
