import { bootstrapApplication } from '@angular/platform-browser';
import { appConfig } from './app/app.config';
import { App } from './app/app';

// Import the JIT compiler to enable runtime component compilation
import '@angular/compiler';

bootstrapApplication(App, appConfig).catch((err) => console.error(err));
