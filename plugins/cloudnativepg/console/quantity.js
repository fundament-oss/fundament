// Kubernetes quantities, for the storage size. Pure: no DOM, no SDK. Copied
// from ceph-rook's console/_shared.js, which another plugin's image cannot
// import from; keep them in step.

const QUANTITY_MULTIPLIERS = {
  Ki: 2 ** 10, Mi: 2 ** 20, Gi: 2 ** 30, Ti: 2 ** 40, Pi: 2 ** 50, Ei: 2 ** 60,
  n: 1e-9, u: 1e-6, m: 1e-3, '': 1, k: 1e3, M: 1e6, G: 1e9, T: 1e12, P: 1e15, E: 1e18,
};

// The Kubernetes Quantity grammar (k8s.io/apimachinery resource.Quantity):
// <signedNumber> followed by a binary SI suffix, a decimal exponent or a
// decimal SI suffix. The exponent is tried first, so "1E3" is 1000, not 1 exa
// followed by garbage.
const QUANTITY_PATTERN = /^([+-]?(?:\d+\.?\d*|\.\d+))(?:([KMGTPE]i)|[eE]([+-]?\d+)|([numkMGTPE]?))$/;

// Whole bytes, rounded up like Quantity.Value(). null for anything outside the
// grammar, and for an exponent too large for a number: "0e999" would
// otherwise be NaN, which passes every range check.
export function parseQuantity(quantity) {
  const match = QUANTITY_PATTERN.exec(quantity);
  if (!match) return null;
  const [, number, binarySI, exponent, decimalSI] = match;
  const value = exponent !== undefined
    ? Number(number) * 10 ** Number(exponent)
    : Number(number) * QUANTITY_MULTIPLIERS[binarySI ?? decimalSI];
  return Number.isFinite(value) ? Math.ceil(value) : null;
}

// The console's memory format (RegionCatalogService.formatMemory): truncated
// to one decimal, without a trailing ".0". Storage adds TiB and PiB above
// 1024 of the unit below.
const SIZE_UNITS = [['PiB', 2 ** 50], ['TiB', 2 ** 40], ['GiB', 2 ** 30]];

function formatSize(bytes) {
  const [unit, size] = SIZE_UNITS.find(([, s]) => bytes >= s) ?? SIZE_UNITS.at(-1);
  const tenths = Math.floor((bytes * 10) / size);
  const whole = Math.floor(tenths / 10);
  const frac = tenths % 10;
  return frac === 0 ? `${whole} ${unit}` : `${whole}.${frac} ${unit}`;
}

// Renders a byte Quantity ("1.5G") as "1.3 GiB". A value outside the Quantity
// grammar is shown as written rather than as a misleading 0 GiB.
export function humanizeQuantity(quantity) {
  if (quantity === undefined || quantity === null || quantity === '') return 'Unknown';
  const bytes = parseQuantity(String(quantity).trim());
  if (bytes === null) return String(quantity);
  const formatted = formatSize(Math.abs(bytes));
  return bytes < 0 && formatted !== '0 GiB' ? `-${formatted}` : formatted;
}
