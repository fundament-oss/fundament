// Grid track lists for the `nldd-table` that renders plugin resources. A plugin
// declares its own columns, so the track list has to be derived rather than
// written out literally like it is on the hand-written tables.

/** A column carries a priority > 0 when it may be dropped on narrow tables. */
export interface TrackColumn {
  priority?: number;
}

const NAME_TRACK = 'minmax(200px, 1fr)';

const VALUE_TRACK = 'minmax(120px, 1fr)';

const ACTIONS_TRACK = '64px';

export function isDroppedWhenNarrow(column: TrackColumn): boolean {
  return !!column.priority && column.priority > 0;
}

/** Name column, one track per printer column, actions column. */
export function tableTracks(columns: readonly TrackColumn[]): string {
  return [NAME_TRACK, ...columns.map(() => VALUE_TRACK), ACTIONS_TRACK].join(' ');
}

/**
 * The same, minus the low-priority columns that `hide-below="lg"` drops, so the
 * track count keeps matching the cells that are actually rendered.
 */
export function narrowTableTracks(columns: readonly TrackColumn[]): string {
  return tableTracks(columns.filter((col) => !isDroppedWhenNarrow(col)));
}
