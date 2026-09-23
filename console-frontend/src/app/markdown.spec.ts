import { describe, expect, it } from 'vitest';
import renderMarkdown from './markdown';

function render(source: string, sectionLevel = 1): HTMLElement {
  const element = document.createElement('div');
  element.innerHTML = renderMarkdown(source, sectionLevel);
  return element;
}

describe('renderMarkdown', () => {
  it('renders headings below the section they sit in', () => {
    const element = render('# Title\n\n## Overview\n\n### Detail');

    expect(element.querySelector('h1')).toBeNull();
    expect([...element.querySelectorAll('h2')].map((h) => h.textContent)).toEqual([
      'Title',
      'Overview',
    ]);
    expect(element.querySelector('h3')?.textContent).toBe('Detail');
  });

  it('renders links, code and lists', () => {
    const element = render('See [docs](https://example.com) and `kubectl`.\n\n- one\n- two');

    const link = element.querySelector('a');
    expect(link?.getAttribute('href')).toBe('https://example.com');
    expect(link?.getAttribute('target')).toBe('_blank');
    expect(element.querySelector('code')?.textContent).toBe('kubectl');
    expect(element.querySelectorAll('ul > li')).toHaveLength(2);
  });

  // Descriptions published as plain text before they were read as markdown
  // must not lose anything that looks like a tag.
  it('shows raw HTML as text', () => {
    const element = render('Create a <CustomResource> object.\n\n<img src="x" onerror="alert(1)">');

    expect(element.querySelector('img')).toBeNull();
    expect(element.textContent).toContain('Create a <CustomResource> object.');
    expect(element.textContent).toContain('<img src="x" onerror="alert(1)">');
  });

  it('does not turn an indented line into code', () => {
    const element = render('Usage:\n\n    run the installer');

    expect(element.querySelector('pre')).toBeNull();
    expect(element.textContent).toContain('run the installer');
  });
});
