// Must come first: the design system is imported transitively from App below,
// and parts of it need browser globals that Node does not have.
import './server-dom-shim';

import { BootstrapContext, bootstrapApplication } from '@angular/platform-browser';
import App from './app/app';
import serverAppConfig from './app/app.config.server';

const bootstrap = (context: BootstrapContext) =>
  bootstrapApplication(App, serverAppConfig, context);

export default bootstrap;
