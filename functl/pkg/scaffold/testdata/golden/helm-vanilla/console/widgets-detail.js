import { loadSdk, renderDefList, renderConditionsTable } from './_shared.js';

await loadSdk();
// fundament.init resolves with the context the host framed this page in,
// including which resource the user navigated to.
const ctx = await fundament.init;
const content = document.getElementById('content');
const heading = document.getElementById('heading');

if (!ctx.resource?.name) {
  content.textContent = 'No Widget selected.';
} else {
  try {
    const item = await fundament.k8s.get({
      group: 'example.com',
      version: 'v1',
      resource: 'widgets',
      name: ctx.resource.name,
      namespace: ctx.resource.namespace,
    });
    heading.textContent = `Widget · ${item.metadata?.name ?? ctx.resource.name}`;
    const meta = {
      Name: item.metadata?.name,
      Namespace: item.metadata?.namespace,
      Created: item.metadata?.creationTimestamp,
    };
    content.innerHTML = `
      <h2 class="plugin-heading">Overview</h2>
      ${renderDefList(meta)}
      <h2 class="plugin-heading">Conditions</h2>
      ${renderConditionsTable(item)}`;
  } catch (err) {
    content.textContent = `Failed to load: ${err?.message ?? err}`;
  }
}
