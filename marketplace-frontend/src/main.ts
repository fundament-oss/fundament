// The components are Dutch first: English for the texts they write themselves.
import { setTranslations } from '@nldd/design-system/translations';
import enUS from '@nldd/design-system/translations/en-US';
import { bootstrapApplication } from '@angular/platform-browser';
import { appConfig } from './app/app.config';
import App from './app/app';

setTranslations(enUS);

// eslint-disable-next-line no-console
bootstrapApplication(App, appConfig).catch((err) => console.error(err));
