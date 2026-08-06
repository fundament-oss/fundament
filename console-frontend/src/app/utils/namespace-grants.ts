/**
 * The shape of namespace access, shared by the two places that hand it out: the
 * member sheet and the add-member form.
 */

/** Picker sentinels: not namespace names, so they cannot collide with one. */
export const NEW_NAMESPACE = '\u0000new';

/** A grant that also covers namespaces made later. */
export const ALL_NAMESPACES = '\u0000all';

/** The all-namespaces grant is not a name, so it needs a label of its own. */
export const namespaceLabel = (namespace: string): string =>
  namespace === ALL_NAMESPACES ? 'All namespaces' : namespace;

/** Same rule the namespaces page enforces. */
export const NAMESPACE_NAME = /^[a-z]([-a-z0-9]*[a-z0-9])?$/;

/**
 * "All" and a hand-picked namespace exclude each other: the first already covers
 * the second. "New namespace…" is not a grant but an instruction to create one,
 * so it combines with either, and the new namespace then falls under "All" by
 * itself.
 */
export const toggleNamespace = (current: string[], namespace: string): string[] => {
  if (current.includes(namespace)) return current.filter((name) => name !== namespace);
  if (namespace === NEW_NAMESPACE) return [...current, NEW_NAMESPACE];
  if (namespace === ALL_NAMESPACES) {
    return [ALL_NAMESPACES, ...current.filter((name) => name === NEW_NAMESPACE)];
  }
  return [...current.filter((name) => name !== ALL_NAMESPACES), namespace];
};
