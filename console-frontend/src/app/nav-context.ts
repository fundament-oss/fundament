import { withinOrganization } from './address';

/**
 * Where you are in the organization, as the menu sees it. Everyone works in one
 * of three places: the organization itself, one of its clusters, or one of the
 * projects on a cluster. The menu shows what belongs to that place and nothing
 * else, and moving into a cluster or a project is what changes it.
 *
 * Read from the address alone, so a link lands in the same place for whoever
 * opens it. A project's address does not name its cluster; the cluster comes
 * from the organization's data once that has loaded (see NavContextService).
 */
export type NavContext =
  | { kind: 'organization' }
  | { kind: 'cluster'; clusterId: string }
  | { kind: 'project'; projectId: string };

/** The address with the organization, the query and the fragment taken off. */
function pathOf(url: string): string {
  return withinOrganization(url.split(/[?#]/)[0]);
}

/**
 * The place an address is in. `/clusters/new` and `/projects/new` are the
 * create sheets over the organization's pages, not a cluster or a project
 * called "new".
 */
export function contextOf(url: string): NavContext {
  const path = pathOf(url);

  const projectId = /^\/projects\/([^/]+)/.exec(path)?.[1];
  if (projectId && projectId !== 'new') return { kind: 'project', projectId };

  const clusterId = /^\/clusters\/([^/]+)/.exec(path)?.[1];
  if (clusterId && clusterId !== 'new') return { kind: 'cluster', clusterId };

  return { kind: 'organization' };
}

/**
 * The address of a place itself, relative to the organization: its menu, with
 * nothing open beside it. Moving into a cluster or a project goes here.
 */
export function contextRoot(context: NavContext): string {
  switch (context.kind) {
    case 'organization':
      return '/';
    case 'cluster':
      return `/clusters/${context.clusterId}`;
    case 'project':
      return `/projects/${context.projectId}`;
    default:
      throw new Error(`unknown navigation context: ${JSON.stringify(context satisfies never)}`);
  }
}

/**
 * A place's overview page, relative to the organization: what opens beside the
 * menu when you move into the place and there is room to show both.
 */
export function overviewOf(context: NavContext): string {
  const root = contextRoot(context);
  return `${root === '/' ? '' : root}/general`;
}

/** Whether an address is a place itself rather than a page inside one. */
export function isContextRoot(url: string): boolean {
  return pathOf(url) === contextRoot(contextOf(url));
}

/**
 * How deep the stacked (narrow) view is for an address: 0 shows the menu of the
 * place you are in, 1 a page inside it. A place's own address has no page to
 * show, so the menu is the whole screen there.
 */
export function depthForPath(url: string): 0 | 1 {
  return isContextRoot(url) ? 0 : 1;
}

/**
 * One step up from a page: the menu of the place it belongs to. Relative to the
 * organization, which is what PageNavService.goTo takes.
 */
export function parentOf(url: string): string {
  return contextRoot(contextOf(url));
}
