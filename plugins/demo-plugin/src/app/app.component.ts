import { Component } from '@angular/core';

/**
 * Minimal root component required for the Angular build to produce a valid app shell.
 * The plugin is loaded by the Fundament host via Native Federation — this component
 * is never rendered in normal use.
 */
@Component({
  selector: 'app-root',
  template: `
    <div style="font-family: sans-serif; padding: 2rem;">
      <h1>Demo plugin</h1>
      <p>
        This plugin is loaded by the Fundament host at
        <a href="http://localhost:4200">http://localhost:4200</a> via Native Federation.
      </p>
      <p>
        Run <code>bun run watch</code> to rebuild on change. The host picks up the
        updated bundle after a page refresh.
      </p>
    </div>
  `,
})
export default class AppComponent {}
