// Types for the NLDD Design System web components this app drives.
//
// @nldd/design-system is a *devDependency*: these are `import type` only, so they
// are erased at build time and no NLDD Design System code enters the bundle. The
// elements are registered at runtime by the shared
// /plugins/sdk/v1/nldd-design-system.js (see loadNlddDesignSystem), which the
// Console serves from its own pinned copy.
//
// The version pinned in package.json must therefore match console-frontend's: it
// describes the bundle the host actually serves, and a mismatch should surface as
// a type error rather than fail silently at runtime.
//
// Importing from the per-component subpaths (rather than the package root) keeps
// TypeScript from pulling in the whole component graph.

import type { NLDDButton } from '@nldd/design-system/button';

export type NlddButton = NLDDButton;
