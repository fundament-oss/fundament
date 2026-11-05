// The lifetimes a new API key can be given, out of the longest the server allows.

/** One choice in the "Expires after" group. `value` is what the server is sent,
 *  except for the two that are not a duration: `NEVER` and `CUSTOM`. */
export interface ExpiryOption {
  value: string;
  label: string;
}

/** No expiry: sent as an empty duration, which the server takes as never. */
export const NEVER = 'never';

/** The duration typed in the field below the group. */
export const CUSTOM = 'custom';

const HOUR = 3_600_000;
const DAY = 24 * HOUR;

/** In hours: the server parses with Go's `time.ParseDuration`, which knows no days. */
const PRESETS: ExpiryOption[] = [
  { value: '168h', label: '7 days' },
  { value: '720h', label: '30 days' },
  { value: '2160h', label: '90 days' },
  { value: '8760h', label: '1 year' },
];

const UNIT_MS: Record<string, number> = {
  ns: 1e-6,
  us: 1e-3,
  µs: 1e-3,
  μs: 1e-3,
  ms: 1,
  s: 1000,
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
  if (maximum % DAY !== 0) return { value: maxExpiresIn, label: maxExpiresIn };
  const days = maximum / DAY;
  return { value: maxExpiresIn, label: days === 1 ? '1 day' : `${days} days` };
}

/**
 * The presets the server allows, then the maximum itself when no preset is
 * it, so the longest lifetime is always one click. With no maximum, a key may
 * never expire, and that is offered instead. A custom duration comes last.
 */
export function expiryOptions(maxExpiresIn: string): ExpiryOption[] {
  const custom = { value: CUSTOM, label: 'Custom…' };
  const maximum = maxExpiresIn ? parseDuration(maxExpiresIn) : null;
  // A maximum this console cannot read is still the server's to enforce: the
  // presets stay, and a refusal names the limit.
  if (maximum === null) {
    return maxExpiresIn
      ? [...PRESETS, custom]
      : [...PRESETS, { value: NEVER, label: 'Never' }, custom];
  }
  const allowed = PRESETS.filter((preset) => (parseDuration(preset.value) ?? Infinity) <= maximum);
  const atMaximum = allowed.some((preset) => parseDuration(preset.value) === maximum);
  return atMaximum
    ? [...allowed, custom]
    : [...allowed, maximumOption(maxExpiresIn, maximum), custom];
}

/** The longest lifetime there is: the maximum, or never when there is none. */
export function defaultExpiry(maxExpiresIn: string): string {
  if (!maxExpiresIn) return NEVER;
  const options = expiryOptions(maxExpiresIn);
  // The option just before "Custom…" is the longest: the maximum, or with an
  // unreadable maximum the longest preset.
  return options[options.length - 2].value;
}
