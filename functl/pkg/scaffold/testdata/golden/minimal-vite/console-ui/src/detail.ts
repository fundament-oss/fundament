import { loadSdk, renderDefList, renderConditionsTable } from './shared.ts';
import type { Widget } from './types.ts';

const sdk = await loadSdk();
// init resolves with the context the host framed this page in, including which
// resource the user navigated to.
const ctx = await sdk.init;
const content = document.getElementById('content') as HTMLElement;
const heading = document.getElementById('heading') as HTMLElement;

if (!ctx.resource?.name) {
  content.textContent = 'No Widget selected.';
} else {
  try {
    const item = await sdk.k8s.get<Widget>({
      group: 'example.com',
      version: 'v1',
      resource: 'widgets',
      name: ctx.resource.name,
      namespace: ctx.resource.namespace ?? undefined,
    });

    heading.textContent = `Widget · ${item.metadata?.name ?? ctx.resource.name}`;
    content.innerHTML = `
      <h2 class="plugin-heading">Overview</h2>
      ${renderDefList({
        Name: item.metadata?.name,
        Namespace: item.metadata?.namespace,
        Created: item.metadata?.creationTimestamp,
      })}
      <h2 class="plugin-heading">Conditions</h2>
      ${renderConditionsTable(item)}`;
  } catch (err) {
    content.textContent = `Failed to load: ${err instanceof Error ? err.message : String(err)}`;
  }
}
