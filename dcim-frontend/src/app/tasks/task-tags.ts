/**
 * A task is filed under tags, and a tag can be a path: `AMS1/R01-3` is the rack
 * in that data center. The slash is the convention other task apps read as a
 * hierarchy, so the structure travels with the tag instead of living in this
 * app's head.
 *
 * Two rules follow from it, and everything here is one of the two:
 *
 * 1. A tag stands for itself and for everything below it. Filtering on `AMS1`
 *    finds a task tagged `AMS1/R01-3`, so you only tag the most specific thing
 *    you know.
 * 2. What older tasks carry as a location text ("AMS1 · R01-3") is read as the
 *    path it describes, so they join the same structure without a migration.
 */

/** What a task carries as a location, as the path it describes. */
export function locationTag(location: string): string {
  const parts = location
    .split('·')
    .map((part) => part.trim())
    .filter(Boolean);
  return parts.join('/');
}

/** Every tag a task is filed under: its own, plus the one its location names. */
export function taskTags(task: { tags: string[]; location: string }): string[] {
  const fromLocation = locationTag(task.location);
  const all = fromLocation ? [fromLocation, ...task.tags] : [...task.tags];
  // A tag that is already covered by a longer path is not a second tag.
  return [...new Set(all)].filter(
    (tag) => !all.some((other) => other !== tag && other.startsWith(`${tag}/`)),
  );
}

/** Whether a task's tag falls under the one being filtered on. */
export function tagMatches(tag: string, filter: string): boolean {
  return tag === filter || tag.startsWith(`${filter}/`);
}

/** One entry in the tag menu: a name, the whole path it stands for, how many
 *  tasks fall under it, and what hangs below it. */
export interface TagNode {
  name: string;
  path: string;
  count: number;
  children: TagNode[];
}

/**
 * The tags in use as a tree, with `roots` always present even when nothing
 * carries them yet: a data center exists whether or not there is work in it.
 */
export function buildTagTree(taskTagLists: string[][], roots: string[] = []): TagNode[] {
  const nodes = new Map<string, TagNode>();

  const ensure = (path: string): TagNode => {
    const existing = nodes.get(path);
    if (existing) return existing;
    const segments = path.split('/');
    const node: TagNode = {
      name: segments[segments.length - 1],
      path,
      count: 0,
      children: [],
    };
    nodes.set(path, node);
    if (segments.length > 1) ensure(segments.slice(0, -1).join('/')).children.push(node);
    return node;
  };

  roots.forEach((root) => ensure(root));
  taskTagLists.forEach((tags) => {
    // A task counts once per entry, and once for every entry above it.
    const counted = new Set<string>();
    tags.forEach((tag) => {
      const segments = tag.split('/');
      segments.forEach((_, i) => counted.add(segments.slice(0, i + 1).join('/')));
    });
    counted.forEach((path) => {
      ensure(path).count += 1;
    });
  });

  const sort = (list: TagNode[]): TagNode[] =>
    list
      .sort((a, b) => a.name.localeCompare(b.name))
      .map((node) => ({ ...node, children: sort(node.children) }));

  const rootNodes = [...nodes.values()].filter((node) => !node.path.includes('/'));
  // The fixed roots first, in the order they were given; the rest alphabetical.
  const fixed = roots.map((root) => nodes.get(root)).filter((node): node is TagNode => !!node);
  const free = sort(rootNodes.filter((node) => !roots.includes(node.path)));
  return [...fixed.map((node) => ({ ...node, children: sort(node.children) })), ...free];
}
