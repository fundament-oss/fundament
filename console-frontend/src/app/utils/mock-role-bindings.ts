/**
 * TEMPORARY, dev only. Roles do not exist in the backend or in the
 * authorization model yet, so this stands in for them while the shape of the
 * screens is being worked out. Derived from the member id so the count on a row
 * and the list in the sheet never disagree. Delete once the API lands.
 */
export interface RoleBinding {
  namespace: string;
  roles: string[];
}

/** The roles you can hand out. Real ones will come from the API. */
export const ALL_ROLES = ['deploy', 'view-pods', 'view-logs'];

const ROLE_SETS = [['view-pods'], ['deploy', 'view-pods'], ['deploy', 'view-pods', 'view-logs']];

/** A stable small number from an arbitrary id. */
const hash = (value: string): number =>
  [...value].reduce((total, character) => total + character.charCodeAt(0), 0);

/** What was handed out during this session, so a grant made in the add form or
 *  the sheet also shows in the count on the list. Gone on reload, like the rest
 *  of this file. */
const granted = new Map<string, RoleBinding[]>();

export const setMockBindings = (memberId: string, bindings: RoleBinding[]): void => {
  granted.set(memberId, bindings);
};

/** Takes the project's real namespaces, so the counts, the list and what is
 *  still available to grant all agree with each other. */
export const mockBindingsFor = (memberId: string, namespaces: string[]): RoleBinding[] => {
  const handedOut = granted.get(memberId);
  if (handedOut) return handedOut;
  if (namespaces.length === 0) return [];
  const seed = hash(memberId);
  const count = seed % (namespaces.length + 1);
  return namespaces.slice(0, count).map((namespace, index) => ({
    namespace,
    roles: ROLE_SETS[(seed + index) % ROLE_SETS.length],
  }));
};
