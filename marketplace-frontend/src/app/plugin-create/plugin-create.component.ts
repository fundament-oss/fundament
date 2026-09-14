import { Component, inject, ChangeDetectionStrategy, CUSTOM_ELEMENTS_SCHEMA } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { ConfigService } from '../config.service';

interface CreateStep {
  command: string;
  title: string;
  body: string;
}

// Static "how to publish a plugin" page, modelled on Stripe's "Build an app"
// flow. Explains the functl CLI pipeline; there is no form here. The three
// commands map onto the Pushed -> Central review -> Publish stages shown by the
// plugin status tracker.
@Component({
  selector: 'app-plugin-create',
  imports: [RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './plugin-create.component.html',
})
export default class PluginCreateComponent {
  private titleService = inject(TitleService);

  // URL of the sibling deployable this page links out to (FUN-20); empty
  // when that area is not deployed, which hides the link.
  protected storefrontUrl = inject(ConfigService).getConfig().storefrontUrl ?? '';

  readonly steps: CreateStep[] = [
    {
      command: 'functl auth login',
      title: 'Authenticate',
      body: 'Signs you in to Fundament with an API key so the CLI can publish to the marketplace registry on your behalf.',
    },
    {
      command: 'functl org set <org-id>',
      title: 'Pick your organization',
      body: 'Publishing is organization-scoped: every listing and version belongs to the organization you select here.',
    },
    {
      command: 'functl plugin publish definition.yaml --image repo@sha256:… --create',
      title: 'Publish a version',
      body: 'Uploads your PluginDefinition manifest with the pushed image digest and registers the version. --create reserves the listing on first publish; published builds can be sideloaded onto your own clusters for testing before you submit them for review.',
    },
  ];

  constructor() {
    this.titleService.setTitle('Build a plugin');
  }
}
