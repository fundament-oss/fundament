// The lifetimes a new API key can be given, out of the longest the server allows.

/** One choice in the "Expires after" group. `value` is what the server is sent,
 *  except for the two that are not a duration: `NEVER` and `CUSTOM`. `ms` is how
 *  long it lasts, which picks the default; null for `CUSTOM`, which has no length
 *  of its own. */
export interface ExpiryOption {
  value: string;
  label: string;
  ms: number | null;
}

/** No expiry: sent as an empty duration, which the server takes as never. */
export const NEVER = 'never';

/** A last day picked in the calendar below the group. */
export const CUSTOM = 'custom';

const SECOND = 1000;
const HOUR = 3_600_000;
const DAY = 24 * HOUR;

/** In hours: the server parses with Go's `time.ParseDuration`, which knows no days. */
const PRESETS: (ExpiryOption & { ms: number })[] = [
  { value: '24h', label: '1 day', ms: DAY },
  { value: '168h', label: '7 days', ms: 7 * DAY },
  { value: '720h', label: '30 days', ms: 30 * DAY },
  { value: '8760h', label: '1 year', ms: 365 * DAY },
];

const UNIT_MS: Record<string, number> = {
  ns: 1e-6,
  us: 1e-3,
  µs: 1e-3,
  μs: 1e-3,
  ms: 1,
  s: SECOND,
  m: 60_000,
  h: HOUR,
};

/** The proto's own rule (apikey.proto), which is what `time.ParseDuration` reads. */
const DURATION = /^[+]?(([0-9]+([.][0-9]*)?|[.][0-9]+)(ns|us|µs|μs|ms|s|m|h))+$/;

const DURATION_PART = /([0-9]+(?:[.][0-9]*)?|[.][0-9]+)(ns|us|µs|μs|ms|s|m|h)/g;

/** A Go duration such as "8760h" or "1h30m" in milliseconds; null when it is not one. */
export function parseDuration(value: string): number | null {
  if (!DURATION.test(value)) return null;
  return Array.from(value.matchAll(DURATION_PART)).reduce(
    (total, [, amount, unit]) => total + Number(amount) * UNIT_MS[unit],
    0,
  );
}

/** A maximum that is no preset, worded like the presets when it is whole days. */
function maximumOption(maxExpiresIn: string, maximum: number): ExpiryOption {
  if (maximum % DAY !== 0) return { value: maxExpiresIn, label: maxExpiresIn, ms: maximum };
  return { value: maxExpiresIn, label: `${maximum / DAY} days`, ms: maximum };
}

/**
 * The presets the server allows, then the maximum itself when no preset is
 * it, so the longest lifetime is always one click. With no maximum, a key may
 * never expire, and that is offered instead. A custom duration comes last.
 *
 * `maxExpiresIn` is null while the maximum is not known yet: the list that
 * brings it has not come, or failed. Never is then held back, as the server
 * may well refuse it.
 */
export function expiryOptions(maxExpiresIn: string | null): ExpiryOption[] {
  const custom = { value: CUSTOM, label: 'Custom…', ms: null };
  if (maxExpiresIn === '') {
    return [...PRESETS, { value: NEVER, label: 'Never', ms: Infinity }, custom];
  }
  const maximum = maxExpiresIn === null ? null : parseDuration(maxExpiresIn);
  // A maximum not known, or one this console cannot read, is still the
  // server's to enforce: the presets stay, and a refusal names the limit.
  if (maxExpiresIn === null || maximum === null) return [...PRESETS, custom];
  const allowed = PRESETS.filter((preset) => preset.ms <= maximum);
  return allowed.some((preset) => preset.ms === maximum)
    ? [...allowed, custom]
    : [...allowed, maximumOption(maxExpiresIn, maximum), custom];
}

/** The longest lifetime on offer: the maximum, or never when there is none. */
export function defaultExpiry(options: ExpiryOption[]): string {
  return options.reduce((longest, option) =>
    (option.ms ?? -1) > (longest.ms ?? -1) ? option : longest,
  ).value;
}

/** The maximum worded like the options, "1 year" rather than "8760h". */
export function maximumLabel(maxExpiresIn: string): string {
  const maximum = parseDuration(maxExpiresIn);
  if (maximum === null) return maxExpiresIn;
  return (PRESETS.find((preset) => preset.ms === maximum) ?? maximumOption(maxExpiresIn, maximum))
    .label;
}

/** A local calendar date as ISO (yyyy-mm-dd), which is what the date picker reads and writes. */
export function isoDate(date: Date): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, '0');
  const d = String(date.getDate()).padStart(2, '0');
  return `${y}-${m}-${d}`;
}

/** The latest day a custom expiry can fall on: the day the longest lifetime
 *  runs out. null when there is no maximum to stop at, or none this console
 *  can read, which leaves the limit to the server. */
export function lastExpiryDate(maxExpiresIn: string | null, now: Date): string | null {
  const maximum = maxExpiresIn ? parseDuration(maxExpiresIn) : null;
  return maximum === null ? null : isoDate(new Date(now.getTime() + maximum));
}

/** When a key picked to last until `date` stops working: at the end of that
 *  local day, or earlier when the maximum runs out during that day. */
export function customExpiryMoment(date: string, maxExpiresIn: string | null, now: Date): Date {
  const [year, month, day] = date.split('-').map(Number);
  const endOfDay = new Date(year, month - 1, day, 23, 59, 59);
  const maximum = maxExpiresIn ? parseDuration(maxExpiresIn) : null;
  if (maximum === null) return endOfDay;
  return new Date(Math.min(endOfDay.getTime(), now.getTime() + maximum));
}

/** The duration the server is sent for `moment`, in whole seconds so it never
 *  goes over the maximum. null when the moment has already passed: the server
 *  refuses zero and a negative duration. */
export function durationUntil(moment: Date, now: Date): string | null {
  const seconds = Math.floor((moment.getTime() - now.getTime()) / SECOND);
  return seconds > 0 ? `${seconds}s` : null;
}
