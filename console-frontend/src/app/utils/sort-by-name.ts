/**
 * The API lists clusters in creation order; the console shows them by name.
 * Anything with a name will do. Returns a new array, the input is left as is.
 */
export default function sortByName<T extends { name: string }>(items: readonly T[]): T[] {
  return [...items].sort((a, b) => a.name.localeCompare(b.name));
}
