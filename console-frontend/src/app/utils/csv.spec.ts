import { slugify, toCsv } from './csv';

describe('toCsv', () => {
  it('quotes every cell and separates rows with CRLF', () => {
    expect(
      toCsv([
        ['a', 'b'],
        ['c', 'd'],
      ]),
    ).toBe('"a","b"\r\n"c","d"');
  });

  it('keeps a comma, a quote and a newline inside their own cell', () => {
    expect(toCsv([['one, two', 'say "hi"', 'line\nbreak']])).toBe(
      '"one, two","say ""hi""","line\nbreak"',
    );
  });

  it('renders an empty document for no rows', () => {
    expect(toCsv([])).toBe('');
  });
});

describe('slugify', () => {
  it('lowercases and hyphenates', () => {
    expect(slugify('All clusters')).toBe('all-clusters');
  });

  it('drops characters a filename should not carry', () => {
    expect(slugify('prod/eu-west (1)')).toBe('prodeu-west-1');
  });

  it('falls back rather than producing an empty name', () => {
    expect(slugify('///')).toBe('export');
  });
});
