import parseSheetRequest from './sheet-request';

describe('parseSheetRequest', () => {
  it("defaults the kind to the opening page's own", () => {
    expect(parseSheetRequest({ view: 'create' }, 'DiskPool')).toMatchObject({
      kind: 'DiskPool',
      view: 'create',
    });
  });

  it('keeps a resource, a kind of its own and params', () => {
    expect(
      parseSheetRequest(
        {
          kind: 'ObjectBucketClaim',
          view: 'detail',
          resource: { name: 'photos', namespace: 'tnt-acme--web' },
          params: { storageClassName: 'cephobj-default' },
        },
        'ObjectStorage',
      ),
    ).toMatchObject({
      kind: 'ObjectBucketClaim',
      view: 'detail',
      resource: { name: 'photos', namespace: 'tnt-acme--web' },
      params: { storageClassName: 'cephobj-default' },
    });
  });

  it.each([
    ['an unknown view', { view: 'list' }],
    ['a view that is not a string', { view: ['create'] }],
    ['a kind that is not a kind', { view: 'create', kind: '../other-plugin' }],
    ['an edit without a resource', { view: 'edit' }],
    ['a detail without a resource', { view: 'detail' }],
    ['a resource name with a slash', { view: 'edit', resource: { name: 'a/b' } }],
    ['a resource that is a string', { view: 'edit', resource: 'pool' }],
    ['a namespace with dots', { view: 'edit', resource: { name: 'a', namespace: 'a.b' } }],
    ['params that are an array', { view: 'create', params: ['a'] }],
    ['a param that is not a string', { view: 'create', params: { a: 1 } }],
    ['a param key with a space', { view: 'create', params: { 'a b': 'c' } }],
    ['a param too long', { view: 'create', params: { a: 'x'.repeat(1025) } }],
    [
      'too many params',
      {
        view: 'create',
        params: Object.fromEntries([...Array(17).keys()].map((i) => [`p${i}`, ''])),
      },
    ],
  ])('refuses %s', (_, message) => {
    expect(parseSheetRequest(message, 'DiskPool')).toBeNull();
  });

  it('numbers each request, so a sheet replaced by another remounts', () => {
    const first = parseSheetRequest({ view: 'create' }, 'DiskPool');
    const second = parseSheetRequest({ view: 'create' }, 'DiskPool');
    expect(second!.seq).toBeGreaterThan(first!.seq);
  });

  it('refuses something that is not an object', () => {
    expect(parseSheetRequest(null, 'DiskPool')).toBeNull();
    expect(parseSheetRequest('create', 'DiskPool')).toBeNull();
  });
});
