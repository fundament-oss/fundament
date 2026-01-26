import { bootstrapApplication } from '@angular/platform-browser';
import { appConfig } from './app/app.config';
import { App } from './app/app';
import 'iconify-icon';

bootstrapApplication(App, appConfig).catch((err) => console.error(err));
