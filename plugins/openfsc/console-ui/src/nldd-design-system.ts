// Types for the NLDD Design System web components this app drives.
//
// @nldd/design-system is a *devDependency*: these are `import type` only, so they
// are erased at build time and no NLDD Design System code enters the bundle. The
// elements are registered at runtime by the shared /plugin-ui/nldd-design-system.js
// (see loadNlddDesignSystem), which the Console serves from its own pinned copy.
//
// The version here must therefore match console-frontend's — it describes the
// bundle the host actually serves. A mismatch shows up as a type error, which is
// the point: previously these shapes were hand-approximated, and an approximation
// that drifts from the real component fails silently at runtime instead.
//
// Importing from the per-component subpaths (rather than the package root) keeps
// TypeScript from pulling in the whole component graph.

import type { NLDDTextField } from '@nldd/design-system/text-field';
import type { NLDDCheckboxField } from '@nldd/design-system/checkbox-field';
import type { NLDDButton } from '@nldd/design-system/button';

export type NlddTextField = NLDDTextField;
export type NlddCheckboxField = NLDDCheckboxField;
export type NlddButton = NLDDButton;
