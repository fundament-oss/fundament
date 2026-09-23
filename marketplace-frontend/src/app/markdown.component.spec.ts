import { describe, expect, it } from 'vitest';
import { TestBed } from '@angular/core/testing';
import MarkdownComponent from './markdown.component';

function render(source: string): HTMLElement {
  const fixture = TestBed.createComponent(MarkdownComponent);
  fixture.componentRef.setInput('source', source);
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('MarkdownComponent', () => {
  it('renders headings one level below the page section', () => {
    const element = render('## Overview\n\nSome **bold** text.\n\n- one\n- two');

    expect(element.querySelector('h3')?.textContent).toBe('Overview');
    expect(element.querySelector('strong')?.textContent).toBe('bold');
    expect(element.querySelectorAll('li')).toHaveLength(2);
  });

  // A `#` must not land at the page section's own level either.
  it('keeps a top-level heading below the page section', () => {
    const element = render('# Title');

    expect(element.querySelector('h2')).toBeNull();
    expect(element.querySelector('h3')?.textContent).toBe('Title');
  });

  it('opens links in a new tab', () => {
    const link = render('[docs](https://example.com)').querySelector('a');

    expect(link?.getAttribute('href')).toBe('https://example.com');
    expect(link?.getAttribute('target')).toBe('_blank');
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
