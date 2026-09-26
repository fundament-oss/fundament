/**
 * The API lists clusters in creation order; the console shows them by name.
 * Returns a new array, the input is left as is.
 */
export default function sortClustersByName<T extends { name: string }>(clusters: readonly T[]): T[] {
  return [...clusters].sort((a, b) => a.name.localeCompare(b.name));
}
