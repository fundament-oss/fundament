import { bootstrapApplication } from '@angular/platform-browser';
import { appConfig } from './app/app.config';
import { App } from './app/app';

// Import Lit components
import './lit-components/fun-button';

bootstrapApplication(App, appConfig).catch((err) => console.error(err));
