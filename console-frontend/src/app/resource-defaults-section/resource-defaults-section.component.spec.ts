import { TestBed } from '@angular/core/testing';
import ResourceDefaultsSectionComponent, {
  modeFor,
  MEMORY_SECTION,
  type DefaultsScope,
  type ResourceMode,
  type ResourceSeed,
} from './resource-defaults-section.component';

const SUGGESTED: ResourceSeed = { request: 64, limit: 128 };

/** The component with its copy, scope and seed bound; both values start unset. */
function build(scope: DefaultsScope = 'cluster', seed: ResourceSeed = SUGGESTED) {
  const fixture = TestBed.createComponent(ResourceDefaultsSectionComponent);
  fixture.componentRef.setInput('copy', MEMORY_SECTION);
  fixture.componentRef.setInput('scope', scope);
  fixture.componentRef.setInput('seed', seed);
  fixture.componentRef.setInput('mode', scope === 'cluster' ? 'none' : 'inherit');
  fixture.detectChanges();
  return fixture;
}

/** These are protected: the template is their only caller. */
interface Internals {
  select(mode: ResourceMode): void;
  edit(field: { set(value: number | undefined): void }, value: number | undefined): void;
  requestMax(): number | undefined;
  limitMax(): number | undefined;
  requestHint(): string | null;
}

describe('ResourceDefaultsSectionComponent on a cluster', () => {
  it('writes the suggested values when Suggested is picked', () => {
    const fixture = build();
    const component = fixture.componentInstance;
    component.request.set(512);

    (component as unknown as Internals).select('suggested');

    expect(component.mode()).toBe('suggested');
    expect(component.request()).toBe(64);
    expect(component.limit()).toBe(128);
  });

  it('leaves values the page already loaded alone when Custom is picked', () => {
    const fixture = build();
    const component = fixture.componentInstance;
    component.request.set(512);

    (component as unknown as Internals).select('custom');

    expect(component.request()).toBe(512);
    // Only the half that was empty gets the suggested value.
    expect(component.limit()).toBe(128);
  });

  it('clears both halves when Not set is picked, which is how the API reads "no default"', () => {
    const fixture = build();
    const component = fixture.componentInstance;
    component.request.set(250);
    component.limit.set(1000);
    fixture.componentRef.setInput('mode', 'custom');

    (component as unknown as Internals).select('none');

    expect(component.mode()).toBe('none');
    expect(component.request()).toBeUndefined();
    expect(component.limit()).toBeUndefined();
  });

  it('stays empty when there is nothing to seed with', () => {
    const fixture = build('cluster', { request: undefined, limit: undefined });
    const component = fixture.componentInstance;

    (component as unknown as Internals).select('custom');

    expect(component.mode()).toBe('custom');
    expect(component.request()).toBeUndefined();
  });

  // The selection describes the values, so it follows them in both directions.
  it('moves to Custom when a value is edited away from the suggested pair', () => {
    const fixture = build();
    const component = fixture.componentInstance;
    (component as unknown as Internals).select('suggested');

    (component as unknown as Internals).edit(component.request, 512);

    expect(component.mode()).toBe('custom');
    expect(component.request()).toBe(512);
  });

  it('moves back to Suggested when the suggested pair is typed back in', () => {
    const fixture = build();
    const component = fixture.componentInstance;
    (component as unknown as Internals).select('custom');
    (component as unknown as Internals).edit(component.request, 512);

    (component as unknown as Internals).edit(component.request, 64);

    expect(component.mode()).toBe('suggested');
  });

  it('renders the number fields for every mode but Not set', () => {
    const fixture = build();
    const host = fixture.nativeElement as HTMLElement;

    expect(host.querySelector('#defaultMemoryRequest')).toBeNull();

    (fixture.componentInstance as unknown as Internals).select('suggested');
    fixture.detectChanges();

    expect(host.querySelector('#defaultMemoryRequest')).not.toBeNull();
  });

  it('has no ceiling: nothing sits above a cluster', () => {
    const fixture = build();
    const internals = fixture.componentInstance as unknown as Internals;

    expect(internals.limitMax()).toBeUndefined();
    expect(internals.requestHint()).toBeNull();
  });
});

describe('ResourceDefaultsSectionComponent on a project', () => {
  /** A project's seed and ceiling are both the cluster's pair. */
  function buildProject(ceiling: ResourceSeed = { request: 100, limit: 500 }) {
    const fixture = build('project', ceiling);
    fixture.componentRef.setInput('ceiling', ceiling);
    fixture.detectChanges();
    return fixture;
  }

  it('offers no Suggested mode: a project starts from its cluster', () => {
    const fixture = buildProject();
    const host = fixture.nativeElement as HTMLElement;

    expect(host.textContent).toContain('Inherit');
    expect(host.textContent).not.toContain('Suggested');
  });

  it('starts a custom pair at the values it was inheriting', () => {
    const fixture = buildProject();
    const component = fixture.componentInstance;

    (component as unknown as Internals).select('custom');

    expect(component.request()).toBe(100);
    expect(component.limit()).toBe(500);
  });

  it('clears both halves when Inherit is picked', () => {
    const fixture = buildProject();
    const component = fixture.componentInstance;
    (component as unknown as Internals).select('custom');

    (component as unknown as Internals).select('inherit');

    expect(component.mode()).toBe('inherit');
    expect(component.request()).toBeUndefined();
    expect(component.limit()).toBeUndefined();
  });

  it('stays on Custom when the cluster pair is typed back in', () => {
    const fixture = buildProject();
    const component = fixture.componentInstance;
    (component as unknown as Internals).select('custom');

    (component as unknown as Internals).edit(component.request, 100);

    expect(component.mode()).toBe('custom');
  });

  it("caps each field at the cluster's value, and the request also at its own limit", () => {
    const fixture = buildProject();
    const component = fixture.componentInstance;
    const internals = component as unknown as Internals;
    (internals as unknown as Internals).select('custom');
    component.limit.set(300);

    expect(internals.limitMax()).toBe(500);
    expect(internals.requestMax()).toBe(100);

    component.limit.set(50);
    expect(internals.requestMax()).toBe(50);
  });

  it("says the ceiling out loud, so it is not only in the field's max", () => {
    const fixture = buildProject();
    const internals = fixture.componentInstance as unknown as Internals;

    expect(internals.requestHint()).toBe("At most 100 MiB, the cluster's default");
  });

  it('has no ceiling where the cluster sets no value', () => {
    const fixture = buildProject({ request: undefined, limit: 500 });
    const internals = fixture.componentInstance as unknown as Internals;

    expect(internals.requestHint()).toBeNull();
  });
});

describe('modeFor', () => {
  it('reads no values at all as the off mode for the scope', () => {
    expect(modeFor('cluster', undefined, undefined, SUGGESTED)).toBe('none');
    expect(modeFor('project', undefined, undefined, SUGGESTED)).toBe('inherit');
  });

  it('reads the suggested values as suggested, on a cluster only', () => {
    expect(modeFor('cluster', 64, 128, SUGGESTED)).toBe('suggested');
    expect(modeFor('project', 64, 128, SUGGESTED)).toBe('custom');
  });

  it('reads anything else as custom, including half a pair', () => {
    expect(modeFor('cluster', 512, 1024, SUGGESTED)).toBe('custom');
    expect(modeFor('cluster', 64, undefined, SUGGESTED)).toBe('custom');
  });
});
