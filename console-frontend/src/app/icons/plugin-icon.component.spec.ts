import { ComponentFixture, TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import PluginIconComponent from './plugin-icon.component';

const LOGO = 'data:image/svg+xml;base64,AA';
const LOGO_DARK = 'data:image/svg+xml;base64,BB';

describe('PluginIconComponent', () => {
  let failProbe = false;
  let probes: { onerror: (() => void) | null }[] = [];

  beforeEach(() => {
    failProbe = false;
    probes = [];
    vi.stubGlobal(
      'Image',
      class {
        onerror: (() => void) | null = null;

        constructor() {
          probes.push(this);
        }

        set src(_: string) {
          if (failProbe) queueMicrotask(() => this.onerror?.());
        }
      },
    );
  });

  afterEach(() => vi.unstubAllGlobals());

  const create = (inputs: Record<string, string>): ComponentFixture<PluginIconComponent> => {
    const fixture = TestBed.createComponent(PluginIconComponent);
    fixture.componentRef.setInput('name', 'grafana');
    Object.entries(inputs).forEach(([key, value]) => fixture.componentRef.setInput(key, value));
    fixture.detectChanges();
    return fixture;
  };

  const imgs = (fixture: ComponentFixture<PluginIconComponent>): HTMLImageElement[] =>
    Array.from((fixture.nativeElement as HTMLElement).querySelectorAll('img'));

  it('renders one img with the backdrop class when only logo is set', () => {
    const found = imgs(create({ logo: LOGO }));
    expect(found).toHaveLength(1);
    expect(found[0].getAttribute('src')).toBe(LOGO);
    expect(found[0].className).toContain('dark:bg-');
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

  it('keeps the tinted mask from the built-in file when logo is empty', () => {
    const fixture = create({});
    const el = fixture.nativeElement as HTMLElement;
    expect(imgs(fixture)).toHaveLength(0);
    expect(el.querySelector('.bg-current')).not.toBeNull();
    expect(el.querySelector('nldd-icon')).toBeNull();
  });

  it('shows the generic plugin icon when the built-in file fails to load', async () => {
    failProbe = true;
    const fixture = create({});
    await fixture.whenStable();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('.bg-current')).toBeNull();
    expect(el.querySelector('nldd-icon')?.getAttribute('name')).toBe('plugin');
  });

  it('ignores a stale probe failure after the name changes', () => {
    const fixture = create({});
    const [stale] = probes;
    fixture.componentRef.setInput('name', 'loki');
    fixture.detectChanges();
    stale.onerror?.();
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).querySelector('.bg-current')).not.toBeNull();
  });

  it('ignores a logo that is not a data:image value', () => {
    const fixture = create({ logo: 'https://example.com/logo.png' });
    expect(imgs(fixture)).toHaveLength(0);
    expect((fixture.nativeElement as HTMLElement).querySelector('.bg-current')).not.toBeNull();
  });
});
