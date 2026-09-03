// Auto-drive runner for the walkthrough. Drives the live app pane through the same
// DOM events its components already handle (e.g. nldd-text-field's `input` CustomEvent
// with detail.value), so no component internals are touched.
//
// The pane is not always the console: a slide with `embed` frames the marketplace
// demo, which is same-origin, so the same runner drives it through that frame's
// document. Everything below therefore goes through the `doc` it is handed rather
// than reaching for the global `document`.
//
// A drive script is sequential by definition: each step must land before the next one
// is dispatched, and typing is emitted one character at a time. Awaiting in a loop is
// the behaviour, not an oversight.
/* eslint-disable no-await-in-loop */
import { DriveStep } from './presentation.model';

const CHAR_MS = 80; // per-character typing delay
const STEP_MS = 900; // default pause between steps

const sleep = (ms: number, signal: AbortSignal) =>
  new Promise<void>((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException('aborted', 'AbortError'));
      return;
    }
    let timer: ReturnType<typeof setTimeout>;
    const onAbort = () => {
      clearTimeout(timer);
      reject(new DOMException('aborted', 'AbortError'));
    };
    timer = setTimeout(() => {
      signal.removeEventListener('abort', onAbort);
      resolve();
    }, ms);
    signal.addEventListener('abort', onAbort, { once: true });
  });

function find(doc: Document, selector: string): Element | null {
  return doc.querySelector(selector);
}

async function waitForElement(
  doc: Document,
  selector: string,
  signal: AbortSignal,
): Promise<Element | null> {
  // The slide navigated moments ago; the target may not be in the DOM yet.
  for (let attempt = 0; attempt < 20; attempt += 1) {
    const el = find(doc, selector);
    if (el) return el;
    await sleep(150, signal);
  }
  return find(doc, selector);
}

// Real typing updates the field before the event reaches a handler, and handlers
// read it back both ways: the console's fields take `detail.value`, the
// marketplace's search reads `event.target.value`. Setting the property first and
// dispatching after satisfies both.
function dispatchInput(el: Element, value: string): void {
  const field: Partial<HTMLInputElement> = el;
  field.value = value;
  el.dispatchEvent(new CustomEvent('input', { detail: { value }, bubbles: true }));
}

async function typeInto(
  doc: Document,
  selector: string,
  value: string,
  signal: AbortSignal,
): Promise<void> {
  const el = await waitForElement(doc, selector, signal);
  if (!el) return;
  for (let i = 1; i <= value.length; i += 1) {
    dispatchInput(el, value.slice(0, i));
    await sleep(CHAR_MS, signal);
  }
}

async function runStep(doc: Document, step: DriveStep, signal: AbortSignal): Promise<void> {
  if (step.wait) {
    await sleep(step.wait, signal);
    return;
  }
  if (step.emit) {
    doc.dispatchEvent(new CustomEvent(step.emit, { bubbles: true }));
    return;
  }
  if (step.click) {
    const el = await waitForElement(doc, step.click, signal);
    (el as HTMLElement | null)?.click();
    return;
  }
  if (step.submit) {
    const el = await waitForElement(doc, step.submit, signal);
    el?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    return;
  }
  if (step.set && step.type) {
    await typeInto(doc, step.set, step.value ?? '', signal);
    return;
  }
  if (step.set && step.select) {
    const el = (await waitForElement(doc, step.set, signal)) as HTMLSelectElement | null;
    if (el) {
      el.value = step.value ?? '';
      el.dispatchEvent(new Event('change', { bubbles: true }));
    }
    return;
  }
  if (step.set && step.check !== undefined) {
    const el = await waitForElement(doc, step.set, signal);
    el?.dispatchEvent(
      new CustomEvent('change', { detail: { checked: step.check }, bubbles: true }),
    );
    return;
  }
  if (step.set) {
    const el = await waitForElement(doc, step.set, signal);
    el?.dispatchEvent(new CustomEvent('change', { detail: { value: step.value }, bubbles: true }));
  }
}

/**
 * Runs a drive script against `doc`, which is the console's own document unless
 * the slide frames the marketplace demo. Resolves when finished or silently on abort.
 */
export default async function runDrive(
  steps: DriveStep[],
  signal: AbortSignal,
  doc: Document = document,
): Promise<void> {
  try {
    // A while loop, not for..of (banned) and not an indexed for (prefer-for-of would
    // then ask for the banned form back).
    let i = 0;
    while (i < steps.length) {
      const step = steps[i];
      if (signal.aborted) return;
      await runStep(doc, step, signal);
      if (!step.wait) await sleep(STEP_MS, signal);
      i += 1;
    }
  } catch (err) {
    if ((err as DOMException)?.name !== 'AbortError') {
      // eslint-disable-next-line no-console
      console.warn('[presentation/drive] step failed', err);
    }
  }
}
