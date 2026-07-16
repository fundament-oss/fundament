// Demo/presentation entrypoint: boots the real app against in-memory mock data and
// mounts the walkthrough overlay. Never imported by the production entrypoint (main.ts).
import { bootstrapApplication } from '@angular/platform-browser';
import { createComponent } from '@angular/core';
import App from './app/app';
import { demoAppConfig } from './app/demo/demo-app.config';
import { PresentationOverlayComponent } from './app/presentation/presentation-overlay.component';
import { PresentationService } from './app/presentation/presentation.service';

bootstrapApplication(App, demoAppConfig)
  .then((appRef) => {
    // Mount the overlay on its own host node outside <app-root>, sharing the app's
    // injector, so the root component (app.ts) stays untouched.
    const host = document.createElement('div');
    document.body.appendChild(host);
    const overlayRef = createComponent(PresentationOverlayComponent, {
      environmentInjector: appRef.injector,
      hostElement: host,
    });
    appRef.attachView(overlayRef.hostView);
    // Force the first render, then start the walkthrough (present-by-default).
    overlayRef.changeDetectorRef.detectChanges();
    appRef.injector.get(PresentationService).initFromUrl();
  })
  // eslint-disable-next-line no-console
  .catch((err) => console.error(err));
