// Demo entrypoint: the real app against in-memory fixtures, so every screen can
// be walked through without a backend and without signing in. Never imported by
// the production entrypoint (main.ts).
import './design-system';
import { bootstrapApplication } from '@angular/platform-browser';
import demoAppConfig from './app/demo/demo-app.config';
import App from './app/app';

bootstrapApplication(App, demoAppConfig)
  // eslint-disable-next-line no-console
  .catch((err) => console.error(err));
