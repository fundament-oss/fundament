import { ComponentFixture, TestBed } from '@angular/core/testing';
import { describe, expect, it } from 'vitest';
import PluginIconComponent from './plugin-icon.component';

const LOGO = 'data:image/svg+xml;base64,AA';
const LOGO_DARK = 'data:image/svg+xml;base64,BB';

describe('PluginIconComponent', () => {
  const create = (inputs: Record<string, string>): ComponentFixture<PluginIconComponent> => {
    const fixture = TestBed.createComponent(PluginIconComponent);
    Object.entries(inputs).forEach(([key, value]) => fixture.componentRef.setInput(key, value));
    fixture.detectChanges();
    return fixture;
  };

  const imgs = (fixture: ComponentFixture<PluginIconComponent>): HTMLImageElement[] =>
    Array.from((fixture.nativeElement as HTMLElement).querySelectorAll('img'));

  const fallback = (fixture: ComponentFixture<PluginIconComponent>): Element | null =>
    (fixture.nativeElement as HTMLElement).querySelector('nldd-icon');

  it('renders one img with the backdrop class when only logo is set', () => {
    const fixture = create({ logo: LOGO });
    const found = imgs(fixture);
    expect(found).toHaveLength(1);
    expect(found[0].getAttribute('src')).toBe(LOGO);
    expect(found[0].className).toContain('dark:bg-');
    expect(fallback(fixture)).toBeNull();
  });

  it('renders both imgs with dark:hidden and hidden dark:block when logoDark is set', () => {
    const found = imgs(create({ logo: LOGO, logoDark: LOGO_DARK }));
    expect(found).toHaveLength(2);
    expect(found[0].className).toContain('dark:hidden');
    expect(found[0].className).not.toContain('dark:bg-');
    expect(found[1].classList.contains('hidden')).toBe(true);
    expect(found[1].classList.contains('dark:block')).toBe(true);
    expect(found[1].getAttribute('src')).toBe(LOGO_DARK);
  });

  it('shows the generic plugin icon when logo is empty', () => {
    const fixture = create({});
    expect(imgs(fixture)).toHaveLength(0);
    expect(fallback(fixture)?.getAttribute('name')).toBe('plugin');
  });

  it('ignores logoDark without a logo', () => {
    const fixture = create({ logoDark: LOGO_DARK });
    expect(imgs(fixture)).toHaveLength(0);
    expect(fallback(fixture)?.getAttribute('name')).toBe('plugin');
  });

  it('ignores a logo that is not a data:image value', () => {
    const fixture = create({ logo: 'https://example.com/logo.png' });
    expect(imgs(fixture)).toHaveLength(0);
    expect(fallback(fixture)?.getAttribute('name')).toBe('plugin');
  });
});
