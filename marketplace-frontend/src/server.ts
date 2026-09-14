// Must come first: loading the Angular app pulls in the design system, and
// parts of it need browser globals that Node does not have.
import './server-dom-shim';

import {
  AngularNodeAppEngine,
  createNodeRequestHandler,
  isMainModule,
  writeResponseToNodeResponse,
} from '@angular/ssr/node';
import express from 'express';
import { readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { AppConfiguration, EMPTY_CONFIGURATION } from './app/config.service';
import ServerContext from './app/server-context';

const browserDistFolder = join(import.meta.dirname, '../browser');

// Long-lived caching is only safe for files whose name changes with their
// contents. Everything else here (index.html, config.json, the favicons) is
// fetched by a stable path and has to be revalidated.
const HASHED_ASSET = /-[A-Z0-9]{8,}\.[a-z0-9]+$/i;

// Reads the first of the candidate paths that exists.
async function readFirstConfig(paths: string[]): Promise<AppConfiguration | undefined> {
  const [head, ...rest] = paths;
  if (!head) {
    return undefined;
  }

  try {
    return JSON.parse(await readFile(head, 'utf8')) as AppConfiguration;
  } catch {
    return readFirstConfig(rest);
  }
}

/**
 * The runtime configuration the browser fetches from
 * `/assets/config/config.json`, read from disk instead: a relative URL has no
 * origin under Node. The API base URLs are then swapped for their in-cluster
 * equivalents where the deployment provides them, so a server render reaches
 * the APIs directly rather than hairpinning back out through the gateway. The
 * browser keeps using the external URLs from the file.
 */
async function loadRuntimeConfig(): Promise<AppConfiguration> {
  // The built server sits next to the browser output; the dev server builds to
  // a temporary directory and leaves the file in `public/`, which is also where
  // the hot-reload container mounts it.
  const candidates = process.env['MARKETPLACE_CONFIG_PATH']
    ? [process.env['MARKETPLACE_CONFIG_PATH']]
    : [
        join(browserDistFolder, 'assets/config/config.json'),
        join(process.cwd(), 'public/assets/config/config.json'),
      ];

  let config = await readFirstConfig(candidates);

  if (!config) {
    // Matches the browser's behaviour when the file is missing: the app still
    // serves, and the views that call an API fail.
    // eslint-disable-next-line no-console
    console.error(`Failed to load configuration from ${candidates.join(' or ')}`);
    config = EMPTY_CONFIGURATION;
  }

  return {
    ...config,
    catalogApiUrl: process.env['CATALOG_API_INTERNAL_URL'] || config.catalogApiUrl,
    registryApiUrl: process.env['REGISTRY_API_INTERNAL_URL'] || config.registryApiUrl,
    adminApiUrl: process.env['ADMIN_API_INTERNAL_URL'] || config.adminApiUrl,
  };
}

const serverContext: ServerContext = { config: await loadRuntimeConfig() };

const app = express();

// Trust the TLS-terminating proxy in front of this server: without it, every
// absolute URL an SSR render produces would claim http:// and the internal
// service host.
const angularApp = new AngularNodeAppEngine({ trustProxyHeaders: true });

// -----------------------------------------------------------------------------
// Security headers
//
// CONNECT_SRC is the external origins of the three marketplace APIs this app
// talks to. The browser calls them directly, so they are listed here even
// though a server render uses the in-cluster URLs.
// -----------------------------------------------------------------------------
const connectSrc = process.env['CONNECT_SRC'] ?? '';

app.use((_req, res, next) => {
  res.setHeader(
    'Content-Security-Policy',
    `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self' ${connectSrc}; frame-ancestors 'none'; object-src 'none'; base-uri 'self';`,
  );
  // Prevent the page from being embedded in a frame on any origin
  res.setHeader('X-Frame-Options', 'DENY');
  // Prevent MIME-type sniffing
  res.setHeader('X-Content-Type-Options', 'nosniff');
  // Send only the origin as referrer for cross-origin requests
  res.setHeader('Referrer-Policy', 'strict-origin-when-cross-origin');
  // Disable access to sensitive browser features not needed by the app
  res.setHeader('Permissions-Policy', 'camera=(), microphone=(), geolocation=(), payment=()');
  next();
});

app.get('/healthz', (_req, res) => {
  res.status(200).send('ok');
});

app.use(
  express.static(browserDistFolder, {
    index: false,
    redirect: false,
    setHeaders: (res, path) => {
      res.setHeader(
        'Cache-Control',
        HASHED_ASSET.test(path) ? 'public, max-age=31536000, immutable' : 'no-cache',
      );
    },
  }),
);

app.use((req, res, next) => {
  // Rendered pages carry the visitor's theme and, on the authenticated areas,
  // their session, so they must never be held in a shared cache.
  res.setHeader('Cache-Control', 'no-store');
  res.setHeader('Vary', 'Cookie');

  angularApp
    .handle(req, serverContext)
    .then((response) => (response ? writeResponseToNodeResponse(response, res) : next()))
    .catch(next);
});

if (isMainModule(import.meta.url)) {
  const port = process.env['PORT'] ?? 4000;
  app.listen(port, () => {
    // eslint-disable-next-line no-console
    console.log(`Node Express server listening on http://localhost:${port}`);
  });
}

/**
 * The request handler used by the Angular CLI (dev-server and during build).
 * The name is part of that contract, so it cannot become a default export.
 */
// eslint-disable-next-line import-x/prefer-default-export
export const reqHandler = createNodeRequestHandler(app);
