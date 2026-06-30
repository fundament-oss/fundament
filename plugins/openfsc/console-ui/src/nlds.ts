// Minimal structural types for the NLDS web components this app drives. The real
// elements are registered at runtime by the shared /plugin-ui/nldd.js bundle
// (loaded via loadNlds); we only describe the few properties we read or set, so
// @nldd/design-system is never a dependency or part of the bundle.

export type NlddTextField = HTMLElement & {
  value: string;
  disabled: boolean;
  required: boolean;
  focus: () => void;
};

export type NlddCheckboxField = HTMLElement & {
  checked: boolean;
  value: string;
};

export type NlddButton = HTMLElement & {
  disabled: boolean;
};
