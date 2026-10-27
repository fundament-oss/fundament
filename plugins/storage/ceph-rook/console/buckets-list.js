// The project's Buckets page: its ObjectBucketClaims across every store. A
// claim's detail and the create form open in a console sheet (see sheet.js).
import { bootPage, emptyStateHtml, errorBox, latestRead, loadingHtml } from './_shared.js';
import { OBC, OBC_KIND, claimListHtml, wireClaimLinks } from './bucket-pages.js';

const list = document.getElementById('list');
list.innerHTML = loadingHtml('Loading buckets…');

const ctx = await bootPage();

// No storageClassName: the create sheet asks for the store.
document.getElementById('create-btn').addEventListener('click', () => {
  fundament.openSheet({ kind: OBC_KIND, view: 'create' });
});

const nextRead = latestRead();

async function showList() {
  const isCurrent = nextRead();
  try {
    // projectId limits the cluster-wide list to the project's namespaces;
    // without it the list would widen to the whole cluster.
    if (!ctx.projectId) throw new Error('this page needs a project');
    const { items } = await fundament.k8s.list({ ...OBC, projectId: ctx.projectId });
    if (!isCurrent()) return;
    if (!items || items.length === 0) {
      list.innerHTML = emptyStateHtml({
        icon: 'cylinder-split-slash',
        text: 'No buckets in this project yet',
      });
      return;
    }
    list.innerHTML = claimListHtml(items, {
      displayNames: ctx.namespaceDisplayNames ?? {},
      storageClass: true,
    });
    wireClaimLinks(list);
  } catch (err) {
    if (isCurrent()) list.innerHTML = errorBox(err);
  }
}

// A bucket created, or a sheet replaced: the list may be out of date.
fundament.onResourcesChanged(() => showList());
await showList();
