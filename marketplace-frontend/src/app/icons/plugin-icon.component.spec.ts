import { ComponentFixture, TestBed } from '@angular/core/testing';
import { PLATFORM_ID } from '@angular/core';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import PluginIconComponent from './plugin-icon.component';

const LOGO = 'data:image/svg+xml;base64,AA';
const LOGO_DARK = 'data:image/svg+xml;base64,BB';

class FakeImage {
  static instances: FakeImage[] = [];

  static failNext = false;

  onerror: (() => void) | null = null;

  constructor() {
    FakeImage.instances.push(this);
  }

  set src(_: string) {
    if (FakeImage.failNext) queueMicrotask(() => this.onerror?.());
  }
}

describe('PluginIconComponent', () => {
  beforeEach(() => {
    FakeImage.instances = [];
    FakeImage.failNext = false;
    vi.stubGlobal('Image', FakeImage);
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

  it('still applies iconColor to the built-in mask', () => {
    const fixture = create({ iconColor: 'text-white' });
    const mask = (fixture.nativeElement as HTMLElement).querySelector('.bg-current');
    expect(mask?.classList.contains('text-white')).toBe(true);
  });

  it('shows the generic plugin icon when the built-in file fails to load', async () => {
    FakeImage.failNext = true;
    const fixture = create({});
    await fixture.whenStable();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('.bg-current')).toBeNull();
    expect(el.querySelector('nldd-icon')?.getAttribute('name')).toBe('plugin');
  });

  it('ignores a stale probe failure after the name changes', () => {
    const fixture = create({});
    const [stale] = FakeImage.instances;
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

  it('renders no probe on the server', () => {
    TestBed.overrideProvider(PLATFORM_ID, { useValue: 'server' });
    create({});
    expect(FakeImage.instances).toHaveLength(0);
  });
});
