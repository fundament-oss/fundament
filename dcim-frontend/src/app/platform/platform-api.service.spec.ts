import { readAllPages } from './platform-api.service';

describe('readAllPages', () => {
  it('follows next_page_token until the last page', async () => {
    const pages: Record<string, { items: number[]; nextPageToken: string }> = {
      '': { items: [1, 2], nextPageToken: 'b' },
      b: { items: [3, 4], nextPageToken: 'c' },
      c: { items: [5], nextPageToken: '' },
    };
    const asked: string[] = [];

    const all = await readAllPages(async (token) => {
      asked.push(token);
      return pages[token];
    });

    expect(all).toEqual([1, 2, 3, 4, 5]);
    expect(asked).toEqual(['', 'b', 'c']);
  });
});
