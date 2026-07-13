// The plugin's @nldd/design-system devDependency supplies *types only* — the
// runtime comes from the shared /plugin-ui/nldd.js the Console serves from its own
// copy (see docs/funs/FUN-18.adoc). The two are therefore describing the same
// bundle, and must be pinned to the same version.
//
// If they drift, nothing fails at build time: the types would describe components
// the host does not actually serve, and the mismatch would only surface as a
// runtime bug in the iframe. Catch it here instead.

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const read = (relative: string): { devDependencies?: Record<string, string>; dependencies?: Record<string, string> } =>
  JSON.parse(readFileSync(fileURLToPath(new URL(relative, import.meta.url)), 'utf8'));

describe('@nldd/design-system pin', () => {
  it('matches the version console-frontend serves at /plugin-ui/nldd.js', () => {
    const plugin = read('../package.json');
    const console_ = read('../../../../console-frontend/package.json');

    const pluginPin = plugin.devDependencies?.['@nldd/design-system'];
    const consolePin = console_.dependencies?.['@nldd/design-system'];

    expect(pluginPin, 'plugin console-ui must pin @nldd/design-system').toBeDefined();
    expect(consolePin, 'console-frontend must pin @nldd/design-system').toBeDefined();
    expect(
      pluginPin,
      `plugin pins ${pluginPin} but the Console serves ${consolePin}; the plugin's types ` +
        `must describe the bundle the host actually serves`,
    ).toBe(consolePin);
  });

  it('is an exact pin, not a range', () => {
    const pluginPin = read('../package.json').devDependencies?.['@nldd/design-system'] ?? '';
    // A ^ or ~ range could resolve to a different minor than the host's on a fresh
    // install, reintroducing exactly the drift this test exists to prevent.
    expect(pluginPin).toMatch(/^\d+\.\d+\.\d+$/);
  });
});
