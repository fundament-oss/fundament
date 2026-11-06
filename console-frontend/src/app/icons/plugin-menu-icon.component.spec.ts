import { TestBed } from '@angular/core/testing';
import { describe, expect, it } from 'vitest';
import PluginMenuIconComponent from './plugin-menu-icon.component';

describe('PluginMenuIconComponent', () => {
  const create = (icon?: string): HTMLElement => {
    const fixture = TestBed.createComponent(PluginMenuIconComponent);
    fixture.componentRef.setInput('icon', icon);
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  };

  const cell = (el: HTMLElement): Element => el.querySelector('nldd-icon-cell') as Element;

  it('renders nldd-icon-cell with the name for an NLDD name', () => {
    const el = create('globe');
    expect(cell(el).getAttribute('icon')).toBe('globe');
    expect(el.querySelector('.bg-current')).toBeNull();
  });

  it('falls back to circle-dashed when undefined', () => {
    expect(cell(create(undefined)).getAttribute('icon')).toBe('circle-dashed');
  });

  it.each(['https://x/y.png', '/img/a.svg', 'icon.svg', 'sha256:abcd', 'data:text/html;base64,AA'])(
    'falls back to circle-dashed for %s',
    (value) => {
      const el = create(value);
      expect(cell(el).getAttribute('icon')).toBe('circle-dashed');
      expect(el.querySelector('.bg-current')).toBeNull();
    },
  );

  it('renders a 20px bg-current mask for a data: value', () => {
    const el = create('data:image/svg+xml;base64,AA');
    const mask = el.querySelector('.bg-current') as HTMLElement;
    expect(mask).not.toBeNull();
    expect(mask.className).toContain('h-5');
    expect(mask.getAttribute('style')).toContain('data:image/svg+xml');
    expect(cell(el).getAttribute('icon')).toBeNull();
  });
});
