import { pluginResourceName } from './plugin-installation.service';

describe('pluginResourceName', () => {
  it('qualifies the plugin name with its publishing organization', () => {
    expect(pluginResourceName('acme', 'cert-manager')).toBe('acme--cert-manager');
  });

  it('slugifies display-style names', () => {
    expect(pluginResourceName('Acme Corp', 'Grafana Alloy')).toBe('acme-corp--grafana-alloy');
  });

  it('keeps the separator when a half slugs to something containing a dash', () => {
    // Slugging the joined string would collapse "--" back to "-" and lose the
    // boundary, making ("acme", "corp-grafana") indistinguishable from
    // ("acme-corp", "grafana").
    expect(pluginResourceName('acme-corp', 'grafana')).toBe('acme-corp--grafana');
    expect(pluginResourceName('acme', 'corp-grafana')).toBe('acme--corp-grafana');
  });
});
