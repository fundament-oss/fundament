// Which audience this build serves (FUN-20): the marketplace ships as three
// deployables over one source tree, mirroring the API split. The catalog
// build is the default; the registry and admin builds swap this file (and the
// route tables) via fileReplacements in angular.json.
export type Variant = 'catalog' | 'registry' | 'admin';

export const VARIANT: Variant = 'catalog';
