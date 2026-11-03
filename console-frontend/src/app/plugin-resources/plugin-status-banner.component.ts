import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  inject,
  input,
} from '@angular/core';
import PluginRegistryService from './plugin-registry.service';
import { installIssue } from '../utils/plugin-install-status';

import '@nldd/design-system/banner';
import '@nldd/design-system/spacer';

interface BannerContent {
  variant: 'warning' | 'critical' | 'accent';
  text: string;
  supportingText: string;
}

/**
 * Says, at the top of a plugin's own screen, that the plugin is not running as
 * it should, and why. The sidebar keeps a plugin's screens whatever its state,
 * so without this a Degraded plugin's errors would read as the screen's own.
 * Renders nothing while the plugin runs.
 */
@Component({
  selector: 'app-plugin-status-banner',
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (content(); as banner) {
      <nldd-banner
        [attr.variant]="banner.variant"
        [attr.text]="banner.text"
        [attr.supporting-text]="banner.supportingText"
      ></nldd-banner>
      <nldd-spacer size="16"></nldd-spacer>
    }
  `,
})
export default class PluginStatusBannerComponent {
  private registry = inject(PluginRegistryService);

  /** The installation name, as in the plugin's routes. */
  installationName = input.required<string>();

  protected content = computed<BannerContent | null>(() => {
    const health = this.registry.installationHealth(this.installationName());
    const issue = installIssue(health);
    if (!health || !issue) return null;

    switch (health.phase) {
      case 'Degraded':
        return {
          variant: 'warning',
          text: 'This plugin is degraded',
          supportingText:
            issue.message ||
            'It is running, but not everything works. This screen may show errors until it recovers.',
        };
      case 'Running':
        // Running, but the controller does not report it ready.
        return {
          variant: 'warning',
          text: 'This plugin is not ready',
          supportingText:
            issue.message ||
            'It is running, but not ready. This screen may show errors until it is.',
        };
      case 'Failed':
        return {
          variant: 'critical',
          text: 'This plugin failed',
          supportingText:
            issue.message ||
            'This screen will not work until the installation is retried or the plugin is reinstalled.',
        };
      default:
        return {
          variant: 'accent',
          text: 'This plugin is still being installed',
          supportingText: issue.message || 'This screen works once the plugin is running.',
        };
    }
  });
}
