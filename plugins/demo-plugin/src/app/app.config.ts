import { ApplicationConfig } from '@angular/core';
import { provideRouter } from '@angular/router';
import { provideHttpClient } from '@angular/common/http';

/** Minimal app config for running the demo plugin as a standalone dev app. */
export const appConfig: ApplicationConfig = {
  providers: [provideRouter([]), provideHttpClient()],
};
