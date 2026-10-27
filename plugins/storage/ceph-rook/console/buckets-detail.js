import { loadSdk, errorBox } from './_shared.js';
import { claimDetailHtml, loadClaim } from './bucket-pages.js';

await loadSdk();
const ctx = await fundament.init;

const content = document.getElementById('content');
const heading = document.getElementById('heading');
const name = ctx.resource?.name;
const namespace = ctx.resource?.namespace;

if (!name || !namespace) {
  content.textContent = 'No bucket selected.';
} else {
  heading.textContent = `Bucket · ${name}`;
  try {
    const { claim, cm } = await loadClaim(namespace, name);
    const namespaceLabel = ctx.namespaceDisplayNames?.[namespace] ?? namespace;
    content.innerHTML = claimDetailHtml(claim, cm, { name, namespace, namespaceLabel });
  } catch (err) {
    content.innerHTML = errorBox(err);
  }
}
