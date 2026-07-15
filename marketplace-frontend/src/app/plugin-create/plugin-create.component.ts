import { Component, inject, ChangeDetectionStrategy, CUSTOM_ELEMENTS_SCHEMA } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TitleService } from '../title.service';

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

  readonly steps: CreateStep[] = [
    {
      command: 'functl plugins create my-plugin',
      title: 'Scaffold a new plugin',
      body: 'Generates a plugin project with a PluginDefinition manifest (metadata, permissions and menu entries) and a starter reconciler you can build on.',
    },
    {
      command: 'functl login',
      title: 'Authenticate',
      body: 'Signs you in to Fundament so the CLI can push builds to the plugin registry on your behalf.',
    },
    {
      command: 'functl plugins push',
      title: 'Push a build',
      body: 'Builds and uploads your plugin image, then registers the version. Pushed builds can be sideloaded onto your own clusters for testing before you submit them for review.',
    },
  ];

  constructor() {
    this.titleService.setTitle('Build a plugin');
  }
}
