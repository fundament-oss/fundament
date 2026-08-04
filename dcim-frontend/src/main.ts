// Per-component subpath imports instead of the package root: the root barrel
// pulls a single 2.6 MB chunk with all ~110 components (including CodeMirror,
// which we never use). Keep this list in sync with the <nldd-*> tags in src/ --
// sub-elements ship with their parent (nldd-table-row via ./table,
// nldd-menu-item via ./menu, nldd-form-field-help-text via ./form-field, etc.).
import '@nldd/design-system/button';
import '@nldd/design-system/button-group';
import '@nldd/design-system/cell';
import '@nldd/design-system/checkbox';
import '@nldd/design-system/dropdown';
import '@nldd/design-system/form';
import '@nldd/design-system/form-field';
import '@nldd/design-system/icon';
import '@nldd/design-system/icon-button';
import '@nldd/design-system/menu';
import '@nldd/design-system/modal-dialog';
import '@nldd/design-system/multi-line-text-field';
import '@nldd/design-system/number-field';
import '@nldd/design-system/page';
import '@nldd/design-system/password-field';
import '@nldd/design-system/popover';
import '@nldd/design-system/radio-button-field';
import '@nldd/design-system/radio-button-group';
import '@nldd/design-system/search-field';
import '@nldd/design-system/segmented-control';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/table';
import '@nldd/design-system/tag';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/text-field';
import '@nldd/design-system/top-title-bar';
import { bootstrapApplication } from '@angular/platform-browser';
import appConfig from './app/app.config';
import App from './app/app';

bootstrapApplication(App, appConfig)
  // eslint-disable-next-line no-console
  .catch((err) => console.error(err));
