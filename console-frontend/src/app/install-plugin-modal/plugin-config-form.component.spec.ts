import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { create } from '@bufbuild/protobuf';
import PluginConfigFormComponent from './plugin-config-form.component';
import { ConfigSchemaEntrySchema, ConfigType } from '../../generated/catalog/v1/catalog_pb';

function buildFixture(schema: Parameters<typeof create<typeof ConfigSchemaEntrySchema>>[1][]) {
  TestBed.configureTestingModule({});
  const fixture = TestBed.createComponent(PluginConfigFormComponent);
  fixture.componentRef.setInput(
    'schema',
    schema.map((entry) => create(ConfigSchemaEntrySchema, entry)),
  );
  fixture.detectChanges();
  return fixture;
}

function build(schema: Parameters<typeof create<typeof ConfigSchemaEntrySchema>>[1][]) {
  return buildFixture(schema).componentInstance;
}

describe('PluginConfigFormComponent', () => {
  it('prefills each field with its declared default', () => {
    const component = build([
      { name: 'MON_COUNT', type: ConfigType.INT, defaultValue: '3' },
    ]);

    expect(component.form.get('MON_COUNT')?.value).toBe('3');
  });

  it('emits only values that differ from the default, plus required keys', () => {
    const component = build([
      { name: 'MON_COUNT', type: ConfigType.INT, defaultValue: '3' },
      { name: 'DEV_LOOP_DEVICES', type: ConfigType.BOOL, defaultValue: 'false' },
      {
        name: 'FAILURE_DOMAIN',
        type: ConfigType.ENUM,
        defaultValue: '',
        required: true,
        values: ['host', 'rack'],
      },
    ]);

    component.form.get('MON_COUNT')?.setValue('1');
    component.form.get('FAILURE_DOMAIN')?.setValue('host');

    let emitted: Record<string, string> | undefined;
    component.confirmed.subscribe((value) => {
      emitted = value;
    });
    component.onSubmit();

    expect(emitted).toEqual({ MON_COUNT: '1', FAILURE_DOMAIN: 'host' });
  });

  it('clearing an optional field omits the key entirely', () => {
    const component = build([
      { name: 'MON_COUNT', type: ConfigType.INT, defaultValue: '3' },
    ]);

    component.form.get('MON_COUNT')?.setValue('');

    let emitted: Record<string, string> | undefined;
    component.confirmed.subscribe((value) => {
      emitted = value;
    });
    component.onSubmit();

    expect(emitted).toEqual({});
  });

  it('blocks submit while a required key is unchosen or an int is malformed', () => {
    const component = build([
      { name: 'MON_COUNT', type: ConfigType.INT, defaultValue: '3' },
      {
        name: 'FAILURE_DOMAIN',
        type: ConfigType.ENUM,
        defaultValue: '',
        required: true,
        values: ['host', 'rack'],
      },
    ]);

    const confirmed = vi.fn();
    component.confirmed.subscribe(confirmed);

    // FAILURE_DOMAIN required but unchosen.
    component.onSubmit();
    expect(confirmed).not.toHaveBeenCalled();

    // FAILURE_DOMAIN chosen but MON_COUNT malformed.
    component.form.get('FAILURE_DOMAIN')?.setValue('host');
    component.form.get('MON_COUNT')?.setValue('three');
    component.onSubmit();
    expect(confirmed).not.toHaveBeenCalled();
  });

  // Content rule: no raw SCREAMING_SNAKE_CASE schema keys on screen. entry.name
  // itself must stay untouched everywhere else (it is the FormControl name and
  // the key onSubmit emits), which every other test in this file already pins.
  it('renders a schema key as a humanized, sentence-case label when no displayName is set', () => {
    const fixture = buildFixture([
      { name: 'MON_COUNT', type: ConfigType.INT, defaultValue: '3' },
    ]);

    expect(fixture.componentInstance.labelFor(fixture.componentInstance.schema()[0])).toBe(
      'Mon count',
    );

    const field = fixture.nativeElement.querySelector('nldd-form-field');
    expect(field?.getAttribute('label')).toBe('Mon count');
  });

  it('renders the curated displayName instead of the humanized key when set', () => {
    const fixture = buildFixture([
      { name: 'MON_COUNT', type: ConfigType.INT, defaultValue: '3', displayName: 'Monitor count' },
    ]);

    expect(fixture.componentInstance.labelFor(fixture.componentInstance.schema()[0])).toBe(
      'Monitor count',
    );

    const field = fixture.nativeElement.querySelector('nldd-form-field');
    expect(field?.getAttribute('label')).toBe('Monitor count');
  });

  it('blocks submit when an int value has 20 digits (does not fit int64)', () => {
    const component = build([
      { name: 'MON_COUNT', type: ConfigType.INT, defaultValue: '3' },
    ]);

    const confirmed = vi.fn();
    component.confirmed.subscribe(confirmed);

    component.form.get('MON_COUNT')?.setValue('99999999999999999999');
    component.onSubmit();

    expect(confirmed).not.toHaveBeenCalled();
  });

  it('blocks submit when a required field is whitespace-only', () => {
    const component = build([
      { name: 'CEPH_IMAGE', type: ConfigType.STRING, defaultValue: '', required: true },
    ]);

    const confirmed = vi.fn();
    component.confirmed.subscribe(confirmed);

    component.form.get('CEPH_IMAGE')?.setValue('   ');
    component.onSubmit();

    expect(confirmed).not.toHaveBeenCalled();
  });

  it('reveals the advanced section on submit when a hidden advanced control is invalid', () => {
    const component = build([
      {
        name: 'CEPH_IMAGE',
        type: ConfigType.STRING,
        defaultValue: '',
        required: true,
        advanced: true,
      },
    ]);

    expect(component.showAdvanced()).toBe(false);
    component.onSubmit();
    expect(component.showAdvanced()).toBe(true);
  });

  describe('onEnter', () => {
    it('does not submit when Enter originates from a non-text-field element', () => {
      const component = build([
        { name: 'MON_COUNT', type: ConfigType.INT, defaultValue: '3' },
      ]);
      const confirmed = vi.fn();
      component.confirmed.subscribe(confirmed);

      component.onEnter({ target: document.createElement('nldd-button') } as unknown as Event);

      expect(confirmed).not.toHaveBeenCalled();
    });

    it('submits when Enter originates from a text field and the form is valid', () => {
      const component = build([
        { name: 'MON_COUNT', type: ConfigType.INT, defaultValue: '3' },
      ]);
      const confirmed = vi.fn();
      component.confirmed.subscribe(confirmed);

      component.onEnter({ target: document.createElement('nldd-text-field') } as unknown as Event);

      expect(confirmed).toHaveBeenCalledTimes(1);
    });
  });
});
