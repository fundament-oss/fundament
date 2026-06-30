// Entry point for the NLDS plugin-UI bundle (public/plugin-ui/nldd.js). Importing
// the package registers every <nldd-*> element as a side effect; plugin iframes
// load the built IIFE via loadNlds() to render real NLDS components.
import '@nldd/design-system';
