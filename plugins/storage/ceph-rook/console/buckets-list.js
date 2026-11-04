import { loadSdk, emptyRow, errorRow, wireRowLinks, navigateToCreate } from './_shared.js';
import { OBC, claimRowHtml } from './bucket-pages.js';

const COLSPAN = 5;
const tbody = document.getElementById('rows');
tbody.innerHTML = emptyRow(COLSPAN, 'Loading…');

await loadSdk();
const ctx = await fundament.init;

document.getElementById('create-btn').addEventListener('click', () => navigateToCreate());

try {
  // projectId limits the cluster-wide list to the project's namespaces;
  // without it the list would widen to the whole cluster.
  if (!ctx.projectId) throw new Error('this page needs a project');
  const { items } = await fundament.k8s.list({ ...OBC, projectId: ctx.projectId });
  if (!items || items.length === 0) {
    tbody.innerHTML = emptyRow(COLSPAN, 'No buckets in this project yet.');
  } else {
    const displayNames = ctx.namespaceDisplayNames ?? {};
    tbody.innerHTML = items.map((c) => claimRowHtml(c, { displayNames, storageClass: true })).join('');
    wireRowLinks(tbody);
  }
} catch (err) {
  tbody.innerHTML = errorRow(COLSPAN, err);
}
