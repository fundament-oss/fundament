// Demo entrypoint: boots the marketplace against in-memory fixtures so it can be
// served as static files from the console demo's origin. Never imported by the
// production entrypoint (main.ts).
//
// The walkthrough embeds this bundle in a same-origin iframe and stays in the
// parent frame, so unlike the console's demo entrypoint there is no overlay to
// mount here — only the bridge the deck uses to move the app between slides.
import { bootstrapApplication } from '@angular/platform-browser';
import { ApplicationRef } from '@angular/core';
import { Router } from '@angular/router';
import App from './app/app';
import demoAppConfig from './app/demo/demo-app.config';

// Message contract with the console's presentation overlay. Duplicated there
// (console-frontend/src/app/presentation/presentation.tokens.ts) because the two
// apps share no code; keep the strings in step.
const NAVIGATE_MESSAGE = 'fundament-demo:navigate';
const NAVIGATED_MESSAGE = 'fundament-demo:navigated';
const READY_MESSAGE = 'fundament-demo:ready';

function installDeckBridge(appRef: ApplicationRef): void {
  if (window.parent === window) return;

  const router = appRef.injector.get(Router);

  window.addEventListener('message', (event: MessageEvent) => {
    // The deck and this bundle are served from one origin, so anything from
    // elsewhere is not the deck and is ignored.
    if (event.origin !== window.location.origin) return;
    const data = event.data as { type?: string; path?: string } | null;
    if (data?.type !== NAVIGATE_MESSAGE || !data.path) return;
    // replaceUrl: a slide change is not a step the viewer took, and an iframe
    // pushing history entries would make the browser's back button walk through
    // marketplace pages instead of leaving the deck.
    const { path } = data;
    router.navigateByUrl(path, { replaceUrl: true }).then(async () => {
      // The deck holds its slide until this lands, so it goes out only once the
      // routed view has rendered — a bare navigation promise resolves before
      // change detection has put the new screen in the document, which is
      // exactly what a drive script would then query.
      await appRef.whenStable();
      window.parent.postMessage({ type: NAVIGATED_MESSAGE, path }, window.location.origin);
    });
  });

  // Bootstrapping finishes well after the iframe's `load` event, so the deck
  // waits for this before it starts a slide's drive script.
  window.parent.postMessage({ type: READY_MESSAGE }, window.location.origin);
}

bootstrapApplication(App, demoAppConfig)
  .then((appRef) => {
    installDeckBridge(appRef);
  })
  // eslint-disable-next-line no-console
  .catch((err) => console.error(err));
