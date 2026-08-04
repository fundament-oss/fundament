import { isDroppedWhenNarrow, narrowTableTracks, tableTracks } from './table-tracks';

describe('table tracks', () => {
  it('brackets the plugin columns with a name and an actions track', () => {
    expect(tableTracks([{}, {}])).toBe(
      'minmax(200px, 1fr) minmax(120px, 1fr) minmax(120px, 1fr) 64px',
    );
  });

  it('still emits name and actions when a resource declares no columns', () => {
    expect(tableTracks([])).toBe('minmax(200px, 1fr) 64px');
  });

  it('drops the low-priority columns on narrow tables', () => {
    const columns = [{ priority: 0 }, { priority: 1 }, {}];
    expect(narrowTableTracks(columns)).toBe(
      'minmax(200px, 1fr) minmax(120px, 1fr) minmax(120px, 1fr) 64px',
    );
  });

  // The track count must equal the cells the row renders, or every column after
  // the dropped one lands in the wrong grid column.
  it('keeps the narrow track count in step with the hidden cells', () => {
    const columns = [{}, { priority: 2 }, { priority: 1 }, { priority: 0 }];
    const kept = columns.filter((col) => !isDroppedWhenNarrow(col));

    expect(narrowTableTracks(columns).split(' 64px')[0].split('minmax').length - 1).toBe(
      kept.length + 1,
    );
  });

  it('treats only a priority above zero as droppable', () => {
    expect(isDroppedWhenNarrow({})).toBe(false);
    expect(isDroppedWhenNarrow({ priority: 0 })).toBe(false);
    expect(isDroppedWhenNarrow({ priority: 1 })).toBe(true);
  });
});
