// Demo entrypoint: boots the marketplace against in-memory fixtures so it can be
// served as static files from the console demo's origin. Never imported by the
// production entrypoint (main.ts).
//
// The walkthrough embeds this bundle in a same-origin iframe and stays in the
// parent frame, so unlike the console's demo entrypoint there is no overlay to
// mount here — only the bridge the deck uses to move the app between slides.
import { bootstrapApplication } from '@angular/platform-browser';
import { Router } from '@angular/router';
import App from './app/app';
import demoAppConfig from './app/demo/demo-app.config';

// Message contract with the console's presentation overlay. Duplicated there
// (console-frontend/src/app/presentation/presentation.tokens.ts) because the two
// apps share no code; keep the strings in step.
const NAVIGATE_MESSAGE = 'fundament-demo:navigate';
const READY_MESSAGE = 'fundament-demo:ready';

function installDeckBridge(router: Router): void {
  if (window.parent === window) return;

  window.addEventListener('message', (event: MessageEvent) => {
    // The deck and this bundle are served from one origin, so anything from
    // elsewhere is not the deck and is ignored.
    if (event.origin !== window.location.origin) return;
    const data = event.data as { type?: string; path?: string } | null;
    if (data?.type !== NAVIGATE_MESSAGE || !data.path) return;
    // replaceUrl: a slide change is not a step the viewer took, and an iframe
    // pushing history entries would make the browser's back button walk through
    // marketplace pages instead of leaving the deck.
    router.navigateByUrl(data.path, { replaceUrl: true });
  });

  // Bootstrapping finishes well after the iframe's `load` event, so the deck
  // waits for this before it starts a slide's drive script.
  window.parent.postMessage({ type: READY_MESSAGE }, window.location.origin);
}

bootstrapApplication(App, demoAppConfig)
  .then((appRef) => {
    installDeckBridge(appRef.injector.get(Router));
  })
  // eslint-disable-next-line no-console
  .catch((err) => console.error(err));
