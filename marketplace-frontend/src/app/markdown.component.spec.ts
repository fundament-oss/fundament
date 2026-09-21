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

  it('opens links in a new tab', () => {
    const link = render('[docs](https://example.com)').querySelector('a');

    expect(link?.getAttribute('href')).toBe('https://example.com');
    expect(link?.getAttribute('target')).toBe('_blank');
  });

  it('strips scripts and event handlers', () => {
    const element = render('<img src="x" onerror="alert(1)"><script>alert(1)</script>');

    expect(element.querySelector('script')).toBeNull();
    expect(element.querySelector('img')?.hasAttribute('onerror')).toBe(false);
  });
});
