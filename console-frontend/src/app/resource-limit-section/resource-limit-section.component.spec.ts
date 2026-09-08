import { TestBed } from '@angular/core/testing';
import ResourceLimitSectionComponent, {
  modeFor,
  MEMORY_SECTION,
  type ResourceMode,
  type ResourceSeed,
} from './resource-limit-section.component';

const PLATFORM_SEED: ResourceSeed = { request: 64, limit: 128 };

/** The component with its copy and seed bound; both values start out unset. */
function build(seed: ResourceSeed = PLATFORM_SEED) {
  const fixture = TestBed.createComponent(ResourceLimitSectionComponent);
  fixture.componentRef.setInput('copy', MEMORY_SECTION);
  fixture.componentRef.setInput('seed', seed);
  fixture.componentRef.setInput('mode', 'unlimited');
  fixture.detectChanges();
  return fixture;
}

/** These are protected: the template is their only caller. */
interface Internals {
  select(mode: ResourceMode): void;
  edit(field: { set(value: number | undefined): void }, value: number | undefined): void;
}

describe('ResourceLimitSectionComponent', () => {
  it('writes the platform values when Defaults is picked', () => {
    const fixture = build();
    const component = fixture.componentInstance;
    component.request.set(512);

    (component as unknown as Internals).select('defaults');

    expect(component.mode()).toBe('defaults');
    expect(component.request()).toBe(64);
    expect(component.limit()).toBe(128);
  });

  it('leaves values the page already loaded alone when Custom is picked', () => {
    const fixture = build();
    const component = fixture.componentInstance;
    component.request.set(512);

    (component as unknown as Internals).select('custom');

    expect(component.request()).toBe(512);
    // Only the half that was empty gets the platform value.
    expect(component.limit()).toBe(128);
  });

  it('clears both halves when Unlimited is picked, which is how the API reads "no limit"', () => {
    const fixture = build();
    const component = fixture.componentInstance;
    component.request.set(250);
    component.limit.set(1000);
    fixture.componentRef.setInput('mode', 'custom');

    (component as unknown as Internals).select('unlimited');

    expect(component.mode()).toBe('unlimited');
    expect(component.request()).toBeUndefined();
    expect(component.limit()).toBeUndefined();
  });

  it('stays empty when the platform itself has nothing to seed with', () => {
    const fixture = build({ request: undefined, limit: undefined });
    const component = fixture.componentInstance;

    (component as unknown as Internals).select('custom');

    expect(component.mode()).toBe('custom');
    expect(component.request()).toBeUndefined();
  });

  // The selection describes the values, so it follows them in both directions.
  it('moves to Custom when a value is edited away from the platform pair', () => {
    const fixture = build();
    const component = fixture.componentInstance;
    (component as unknown as Internals).select('defaults');

    (component as unknown as Internals).edit(component.request, 512);

    expect(component.mode()).toBe('custom');
    expect(component.request()).toBe(512);
  });

  it('moves back to Defaults when the platform pair is typed back in', () => {
    const fixture = build();
    const component = fixture.componentInstance;
    (component as unknown as Internals).select('custom');
    (component as unknown as Internals).edit(component.request, 512);

    (component as unknown as Internals).edit(component.request, 64);

    expect(component.mode()).toBe('defaults');
  });

  it('renders the number fields for every mode but Unlimited', () => {
    const fixture = build();
    const host = fixture.nativeElement as HTMLElement;

    expect(host.querySelector('#defaultMemoryRequest')).toBeNull();

    (fixture.componentInstance as unknown as Internals).select('defaults');
    fixture.detectChanges();

    expect(host.querySelector('#defaultMemoryRequest')).not.toBeNull();
  });
});

describe('modeFor', () => {
  it('reads no values at all as unlimited', () => {
    expect(modeFor(undefined, undefined, PLATFORM_SEED)).toBe('unlimited');
  });

  it('reads the platform values as defaults', () => {
    expect(modeFor(64, 128, PLATFORM_SEED)).toBe('defaults');
  });

  it('reads anything else as custom, including half a pair', () => {
    expect(modeFor(512, 1024, PLATFORM_SEED)).toBe('custom');
    expect(modeFor(64, undefined, PLATFORM_SEED)).toBe('custom');
  });
});
