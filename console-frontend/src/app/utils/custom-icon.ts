// Only the data: URIs the server produces are rendered.
const CUSTOM_ICON_PREFIXES = ['data:image/svg+xml;base64,', 'data:image/png;base64,'];

export const isCustomIcon = (value: string | undefined): boolean =>
  !!value && CUSTOM_ICON_PREFIXES.some((prefix) => value.startsWith(prefix));

export default isCustomIcon;
