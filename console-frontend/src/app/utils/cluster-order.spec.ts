import sortClustersByName from './cluster-order';

describe('sortClustersByName', () => {
  it('orders by name and leaves the input untouched', () => {
    const input = [{ name: 'charlie' }, { name: 'alpha' }, { name: 'bravo' }];

    expect(sortClustersByName(input).map((c) => c.name)).toEqual(['alpha', 'bravo', 'charlie']);
    expect(input.map((c) => c.name)).toEqual(['charlie', 'alpha', 'bravo']);
  });
});
