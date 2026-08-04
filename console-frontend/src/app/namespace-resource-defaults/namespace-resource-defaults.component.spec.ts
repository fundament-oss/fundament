import { TestBed } from '@angular/core/testing';
import NamespaceResourceDefaultsComponent, {
  type NamespaceDefaults,
} from './namespace-resource-defaults.component';

const PLATFORM_DEFAULTS: NamespaceDefaults = {
  defaultMemoryRequestMi: 64,
  defaultMemoryLimitMi: 128,
  defaultCpuRequestM: 100,
  defaultCpuLimitM: 500,
};

/** The component with `defaults` bound; every value starts out unset. */
function build(defaults: NamespaceDefaults = PLATFORM_DEFAULTS) {
  const fixture = TestBed.createComponent(NamespaceResourceDefaultsComponent);
  fixture.componentRef.setInput('defaults', defaults);
  fixture.componentRef.setInput('memoryLimited', false);
  fixture.componentRef.setInput('cpuLimited', false);
  fixture.detectChanges();
  return fixture;
}

/** `toggleMemory`/`toggleCpu` are protected: the template is their only caller. */
interface Toggles {
  toggleMemory(limited: boolean): void;
  toggleCpu(limited: boolean): void;
}

describe('NamespaceResourceDefaultsComponent', () => {
  it('seeds an empty pair with the platform defaults when switched to limited', () => {
    const fixture = build();
    const component = fixture.componentInstance;

    (component as unknown as Toggles).toggleMemory(true);

    expect(component.memoryLimited()).toBe(true);
    expect(component.memoryRequestMi()).toBe(64);
    expect(component.memoryLimitMi()).toBe(128);
  });

  it('leaves values the page already loaded alone', () => {
    const fixture = build();
    const component = fixture.componentInstance;
    component.memoryRequestMi.set(512);

    (component as unknown as Toggles).toggleMemory(true);

    expect(component.memoryRequestMi()).toBe(512);
    // Only the half that was empty gets the platform default.
    expect(component.memoryLimitMi()).toBe(128);
  });

  it('clears both halves when switched to unlimited, which is how the API reads "no limit"', () => {
    const fixture = build();
    const component = fixture.componentInstance;
    component.cpuRequestM.set(250);
    component.cpuLimitM.set(1000);
    fixture.componentRef.setInput('cpuLimited', true);

    (component as unknown as Toggles).toggleCpu(false);

    expect(component.cpuLimited()).toBe(false);
    expect(component.cpuRequestM()).toBeUndefined();
    expect(component.cpuLimitM()).toBeUndefined();
  });

  it('stays empty when the platform itself has no defaults to seed with', () => {
    const fixture = build({
      defaultMemoryRequestMi: undefined,
      defaultMemoryLimitMi: undefined,
      defaultCpuRequestM: undefined,
      defaultCpuLimitM: undefined,
    });
    const component = fixture.componentInstance;

    (component as unknown as Toggles).toggleMemory(true);

    expect(component.memoryLimited()).toBe(true);
    expect(component.memoryRequestMi()).toBeUndefined();
  });

  it('switches memory and CPU independently', () => {
    const fixture = build();
    const component = fixture.componentInstance;

    (component as unknown as Toggles).toggleMemory(true);

    expect(component.cpuLimited()).toBe(false);
    expect(component.cpuRequestM()).toBeUndefined();
  });

  it('renders the number fields only while a pair is limited', () => {
    const fixture = build();
    const host = fixture.nativeElement as HTMLElement;

    expect(host.querySelector('#defaultMemoryRequest')).toBeNull();

    (fixture.componentInstance as unknown as Toggles).toggleMemory(true);
    fixture.detectChanges();

    expect(host.querySelector('#defaultMemoryRequest')).not.toBeNull();
    expect(host.querySelector('#defaultCpuRequest')).toBeNull();
  });
});
