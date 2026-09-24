import { resolveHandoff } from './login-handoff';
import { AppConfiguration } from '../config.service';

const DEVELOPER_URL = 'https://marketplace-registry.fundament.localhost:8443';

const config: AppConfiguration = {
  authnApiUrl: '',
  organizationApiUrl: '',
  marketplaceApiUrl: '',
  kubeApiProxyUrl: '',
  pluginProxyUrl: '',
  developerUrl: DEVELOPER_URL,
};

describe('resolveHandoff', () => {
  it('returns the page the portal asked to come back to', () => {
    const handoff = resolveHandoff('marketplace-registry', '/manage/my-plugin', config);

    expect(handoff?.url).toBe(`${DEVELOPER_URL}/manage/my-plugin`);
  });

  it('says on the page why the visitor is here', () => {
    expect(resolveHandoff('marketplace-registry', '/manage', config)?.description).toContain(
      'Developer Portal',
    );
  });

  it('keeps a query and a fragment', () => {
    const handoff = resolveHandoff('marketplace-registry', '/manage?tab=drafts#top', config);

    expect(handoff?.url).toBe(`${DEVELOPER_URL}/manage?tab=drafts#top`);
  });

  it('falls back to the front page without a path', () => {
    expect(resolveHandoff('marketplace-registry', null, config)?.url).toBe(`${DEVELOPER_URL}/`);
  });

  // An ordinary console login: nothing to hand off to, so the page behaves as
  // it always did.
  it('is not a hand-off without an app', () => {
    expect(resolveHandoff(null, '/manage', config)).toBeNull();
  });

  it('is not a hand-off for an app this console does not know', () => {
    expect(resolveHandoff('evil', '/manage', config)).toBeNull();
  });

  // Known, but not deployed in this environment, so there is nowhere to go
  // back to. The visitor still has a console account.
  it('is not a hand-off when the app has no URL configured', () => {
    expect(
      resolveHandoff('marketplace-registry', '/manage', { ...config, developerUrl: '' }),
    ).toBeNull();
  });

  // The whole reason the request names an app rather than a URL: nothing a
  // caller writes can move the destination off the configured origin.
  describe('cannot be steered off the configured origin', () => {
    const offOrigin = [
      '//evil.example/manage',
      '/\\evil.example/manage',
      '\\\\evil.example/manage',
      'https://evil.example/manage',
      'evil.example',
      '',
    ];

    offOrigin.forEach((path) => {
      it(`ignores ${JSON.stringify(path)}`, () => {
        const handoff = resolveHandoff('marketplace-registry', path, config);

        expect(handoff?.url).toBe(`${DEVELOPER_URL}/`);
      });
    });
  });
});
