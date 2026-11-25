import {
  bgpText,
  componentTag,
  componentType,
  formatDuration,
  formatKilowatts,
  formatMilliseconds,
  formatRate,
  formatSeconds,
  formatTimestamp,
  formatWatts,
  ipText,
  issueSeverityTag,
  machineIssueTag,
  machineName,
  machineRole,
  machineStateTag,
  powerText,
  prefixText,
  psuText,
  syncTag,
  tokenTag,
  tokenText,
} from './metal-format';
import type { ComponentStatus, Machine } from './metal-metrics';

const machine = (overrides: Partial<Machine> = {}): Machine => ({
  id: 'machine-1',
  name: 'ams1-c1-large-01',
  partition: 'ams1',
  size: 'c1-large-x86',
  state: 'available',
  role: 'machine',
  clusterTag: 'cl-utrecht-prod',
  imageId: 'ubuntu-24.04',
  powerWatts: 300,
  powerState: 'on',
  psuTotal: 2,
  psuHealthy: 2,
  issues: [],
  ...overrides,
});

const component = (overrides: Partial<ComponentStatus> = {}): ComponentStatus => ({
  id: 'component-1',
  type: 'api',
  identifier: 'metal-api',
  up: true,
  tokenLifetimeSeconds: 30 * 86400,
  ...overrides,
});

describe('formatters', () => {
  it('writes watts, kilowatts and milliseconds', () => {
    expect(formatWatts(1234.6)).toBe('1235 W');
    expect(formatKilowatts(12345)).toBe('12.3 kW');
    expect(formatMilliseconds(1234.6)).toBe('1235 ms');
  });

  it('writes a latency in the unit that keeps it readable', () => {
    expect(formatSeconds(0.1916)).toBe('192 ms');
    expect(formatSeconds(1.5)).toBe('1.5 s');
    expect(formatSeconds(2.17)).toBe('2.17 s');
    expect(formatSeconds(12.34)).toBe('12.3 s');
  });

  it('writes a rate', () => {
    expect(formatRate(230.4)).toBe('230/min');
    expect(formatRate(3.24)).toBe('3.2/min');
  });

  it('writes a remaining lifetime in its largest whole unit', () => {
    expect(formatDuration(30 * 86400)).toBe('30 d');
    expect(formatDuration(8 * 3600)).toBe('8 h');
    expect(formatDuration(40 * 60)).toBe('40 min');
  });

  it('writes the snapshot clock in UTC', () => {
    expect(formatTimestamp(new Date('2026-08-12T09:00:00Z'))).toBe('2026-08-12 09:00 UTC');
  });
});

describe('machineStateTag', () => {
  it('names every state', () => {
    expect(machineStateTag('available')).toEqual({ text: 'Available', color: 'success' });
    expect(machineStateTag('tainted')).toEqual({ text: 'Tainted', color: 'warning' });
    expect(machineStateTag('locked')).toEqual({ text: 'Locked', color: 'neutral' });
    expect(machineStateTag('other')).toEqual({ text: 'Other', color: 'neutral' });
  });
});

describe('issueSeverityTag', () => {
  it('names every severity', () => {
    expect(issueSeverityTag('critical')).toEqual({ text: 'Critical', color: 'critical' });
    expect(issueSeverityTag('major')).toEqual({ text: 'Major', color: 'warning' });
    expect(issueSeverityTag('minor')).toEqual({ text: 'Minor', color: 'neutral' });
  });
});

describe('machineIssueTag', () => {
  it('says nothing when there is nothing wrong', () => {
    expect(machineIssueTag(machine())).toBeNull();
  });

  it('counts the issues in the colour of the worst one', () => {
    const issues = [
      { id: 'bmc-info-outdated', severity: 'major' as const, description: '', refUrl: '' },
      { id: 'last-event-error', severity: 'minor' as const, description: '', refUrl: '' },
    ];
    expect(machineIssueTag(machine({ issues }))).toEqual({ text: '2', color: 'warning' });
    expect(
      machineIssueTag(
        machine({
          issues: [
            { id: 'failed-machine-reclaim', severity: 'critical', description: '', refUrl: '' },
          ],
        }),
      ),
    ).toEqual({ text: '1', color: 'critical' });
  });

  it('keeps a machine with only minor issues neutral, as the issue itself is', () => {
    const issues = [
      { id: 'last-event-error', severity: 'minor' as const, description: '', refUrl: '' },
    ];
    expect(machineIssueTag(machine({ issues }))).toEqual({ text: '1', color: 'neutral' });
  });
});

describe('component and switch tags', () => {
  it('marks a component and a switch that are not healthy', () => {
    expect(componentTag(component())).toEqual({ text: 'Up', color: 'success' });
    expect(componentTag(component({ up: false }))).toEqual({ text: 'Down', color: 'critical' });
    expect(syncTag(false)).toEqual({ text: 'In sync', color: 'success' });
    expect(syncTag(true)).toEqual({ text: 'Failing', color: 'critical' });
  });

  it('warns about a token that expires within a week', () => {
    expect(tokenTag(30 * 86400)).toBeNull();
    expect(tokenTag(7 * 86400)).toBeNull();
    expect(tokenTag(2 * 86400)).toEqual({ text: 'Expires soon', color: 'warning' });
    expect(tokenTag(null)).toBeNull();
    expect(tokenTag(0)).toEqual({ text: 'Expired', color: 'critical' });
    expect(tokenTag(-3 * 86400)).toEqual({ text: 'Expired', color: 'critical' });
  });
});

describe('table cells', () => {
  it('keeps the NOTALLOCATED sentinel out of the table', () => {
    expect(machineName(machine({ name: '' }))).toBe('Not allocated');
    expect(machineRole(machine({ role: 'none' }))).toBe('');
    expect(machineRole(machine({ role: 'firewall' }))).toBe('firewall');
  });

  it('leaves a missing reading empty rather than zero', () => {
    expect(powerText(machine({ powerWatts: null }))).toBe('');
    expect(powerText(machine({ powerWatts: 300 }))).toBe('300 W');
    expect(psuText(machine({ psuTotal: 0, psuHealthy: 0 }))).toBe('');
    expect(psuText(machine({ psuTotal: 2, psuHealthy: 1 }))).toBe('1 / 2');
  });

  it('writes shared capacity and BGP sessions', () => {
    expect(
      ipText({
        id: 'n1',
        name: 'internet-ams1',
        partition: 'ams1',
        ipsUsed: 210,
        ipsAvailable: 44,
        prefixesUsed: 3,
        prefixesAvailable: 2,
      }),
    ).toBe('210 / 254');
    expect(
      prefixText({
        id: 'n1',
        name: 'internet-ams1',
        partition: 'ams1',
        ipsUsed: 210,
        ipsAvailable: 44,
        prefixesUsed: 3,
        prefixesAvailable: 2,
      }),
    ).toBe('3 / 5');
    expect(
      bgpText({
        name: 'leaf-ams1-01',
        partition: 'ams1',
        rack: 'R01-1',
        metalCoreUp: true,
        syncFailing: false,
        os: 'SONiC',
        metalCoreVersion: '0.9.1',
        bgpEstablished: 7,
        bgpTotal: 8,
      }),
    ).toBe('7 / 8');
  });

  it('falls back to the identifier for a component without a type', () => {
    expect(componentType(component())).toBe('api');
    expect(componentType(component({ type: '' }))).toBe('metal-api');
    expect(tokenText(component())).toBe('30 d');
    expect(tokenText(component({ tokenLifetimeSeconds: null }))).toBe('');
    expect(tokenText(component({ tokenLifetimeSeconds: -3 * 86400 }))).toBe('Expired');
  });
});
