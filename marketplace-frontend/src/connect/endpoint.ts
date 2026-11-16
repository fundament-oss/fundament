/**
 * A path under a configured API base URL. Joined under it rather than resolved
 * against it, which would drop a path prefix the API may be served under:
 * `/api/authn` and `login` make `/api/authn/login`, not `/login`. The base is
 * itself resolved against this page, so a root-relative one works too.
 */
export default function endpoint(baseUrl: string | undefined, path: string): URL {
  const configured = baseUrl ?? '';
  const base = new URL(
    configured.endsWith('/') ? configured : `${configured}/`,
    window.location.origin,
  );
  return new URL(path, base);
}
