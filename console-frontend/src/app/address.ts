/**
 * Every address in the console names the organization it belongs to:
 *
 *     /organizations/gemeente-fundament/clusters
 *     /organizations/gemeente-fundament/projects/pr-burgerzaken/limits
 *
 * The organization is what everything else hangs under, so it sits in front of
 * the address rather than in a header nobody can see. An address you paste into
 * a message lands the other person in the organization you were reading, and a
 * second tab can hold a second organization without the two fighting over one
 * setting.
 *
 * By its name, not its id: the id is a UUID, and the name is unique, cannot be
 * changed and reads as a word. An address is written once and then shared and
 * bookmarked, so it wants both of those.
 *
 * Under it the address reads as it always did, which is what these three
 * functions are for: they put an address inside an organization, take it back
 * out, and read which organization an address names.
 */

const PREFIX = '/organizations/';

/** The organization an address names, or null for one that names none. */
export function organizationOf(url: string): string | null {
  if (!url.startsWith(PREFIX)) return null;
  return url.slice(PREFIX.length).split(/[/?#]/)[0] || null;
}

/** What an address says about the page, with the organization taken off. */
export function withinOrganization(url: string): string {
  const id = organizationOf(url);
  if (!id) return url;

  const rest = url.slice(PREFIX.length + id.length);
  return rest.startsWith('/') ? rest : `/${rest}`;
}

/**
 * An address inside an organization, from one relative to it. Takes an address
 * that already names an organization too, so passing one through twice is
 * harmless.
 */
export function inOrganization(organization: string | null, path = '/'): string {
  if (!organization) return path;

  const within = withinOrganization(path);
  if (within === '/') return `${PREFIX}${organization}`;

  // The organization's own address with something behind it: no slash between
  // the two, or the address bar reads `/organizations/gemeente-fundament/?present=1`.
  return `${PREFIX}${organization}${within.replace(/^\/(?=[?#])/, '')}`;
}
