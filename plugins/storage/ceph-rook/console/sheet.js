// The one page every sheet loads. definition.yaml points each kind's create
// and edit, and the bucket pages, at sheet.html, and the console opens it
// with the kind and view it was asked for, so the form is picked here rather
// than by an HTML file per form. The create forms also serve the routed
// create page, which loads this same file.
import { bootPage, errorBox } from './_shared.js';
import {
  consumerCreatePage,
  consumerEditPage,
  BLOCKSTORAGE,
  FILESTORAGE,
  OBJECTSTORAGE,
} from './consumer-pages.js';
import { diskPoolCreatePage, diskPoolEditPage } from './diskpool-forms.js';
import { bucketCreatePage, bucketDetailPage } from './bucket-pages.js';

const consumer = (cfg) => ({
  create: () => consumerCreatePage(cfg),
  edit: (ctx) => consumerEditPage(cfg, ctx),
});

// crdKind -> view -> page; each page gets the init context.
const PAGES = {
  DiskPool: { create: diskPoolCreatePage, edit: diskPoolEditPage },
  BlockStorage: consumer(BLOCKSTORAGE),
  FileStorage: consumer(FILESTORAGE),
  ObjectStorage: consumer(OBJECTSTORAGE),
  ObjectBucketClaim: { create: bucketCreatePage, detail: bucketDetailPage },
};

async function main() {
  const content = document.getElementById('content');
  // The forms are built from the NLDD bundle; bootPage says so in plain text
  // when it does not load.
  const ctx = await bootPage();
  const page = PAGES[ctx.crdKind]?.[ctx.view];
  if (!page) {
    content.innerHTML = errorBox(`no ${ctx.view} form for ${ctx.crdKind}`, 'Cannot open');
    return;
  }
  await page(ctx);
}

await main();
