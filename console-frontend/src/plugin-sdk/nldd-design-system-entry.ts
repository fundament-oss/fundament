// Entry point for the NLDD Design System plugin-UI bundle
// (public/plugin-ui/nldd-design-system.js). Importing the package registers every
// <nldd-*> element as a side effect; plugin iframes load the built IIFE via
// loadNlddDesignSystem() to render real components.
import '@nldd/design-system';
import { setTranslations } from '@nldd/design-system/translations';
import enUS from '@nldd/design-system/translations/en-US';

// The components are Dutch first, and plugin pages are English, like the
// Console around them. A `translations` set on an element still wins.
setTranslations(enUS);
