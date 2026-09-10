import { REQUEST, inject } from '@angular/core';

// The authenticated APIs are called with the visitor's session. In the
// browser that means asking fetch to send cookies cross-origin; during a
// server render there is no cookie jar and `credentials: 'include'` does
// nothing, so the visitor's Cookie header is forwarded from the incoming
// request instead. Must be called in an injection context.
// eslint-disable-next-line import-x/prefer-default-export
export function credentialedFetch(): typeof fetch {
  const request = inject(REQUEST, { optional: true });

  if (!request) {
    return (input, init) => fetch(input, { ...init, credentials: 'include' });
  }

  const cookie = request.headers.get('cookie');

  return (input, init) => {
    const headers = new Headers(init?.headers);
    if (cookie) {
      headers.set('cookie', cookie);
    }
    return fetch(input, { ...init, headers });
  };
}
