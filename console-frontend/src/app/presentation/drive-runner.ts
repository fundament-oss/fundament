// Auto-drive runner for the walkthrough. Drives the live app pane through the same
// DOM events its components already handle (e.g. nldd-text-field's `input` CustomEvent
// with detail.value), so no component internals are touched.
import { DriveStep } from './presentation.model';

const CHAR_MS = 80; // per-character typing delay
const STEP_MS = 900; // default pause between steps

const sleep = (ms: number, signal: AbortSignal) =>
  new Promise<void>((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException('aborted', 'AbortError'));
      return;
    }
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', onAbort);
      resolve();
    }, ms);
    const onAbort = () => {
      clearTimeout(timer);
      reject(new DOMException('aborted', 'AbortError'));
    };
    signal.addEventListener('abort', onAbort, { once: true });
  });

function find(selector: string): Element | null {
  return document.querySelector(selector);
}

async function waitForElement(selector: string, signal: AbortSignal): Promise<Element | null> {
  // The slide navigated moments ago; the target may not be in the DOM yet.
  for (let attempt = 0; attempt < 20; attempt += 1) {
    const el = find(selector);
    if (el) return el;
    await sleep(150, signal);
  }
  return find(selector);
}

function dispatchInput(el: Element, value: string): void {
  el.dispatchEvent(new CustomEvent('input', { detail: { value }, bubbles: true }));
}

async function typeInto(selector: string, value: string, signal: AbortSignal): Promise<void> {
  const el = await waitForElement(selector, signal);
  if (!el) return;
  for (let i = 1; i <= value.length; i += 1) {
    dispatchInput(el, value.slice(0, i));
    await sleep(CHAR_MS, signal);
  }
}

async function runStep(step: DriveStep, signal: AbortSignal): Promise<void> {
  if (step.wait) {
    await sleep(step.wait, signal);
    return;
  }
  if (step.click) {
    const el = await waitForElement(step.click, signal);
    (el as HTMLElement | null)?.click();
    return;
  }
  if (step.submit) {
    const el = await waitForElement(step.submit, signal);
    el?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    return;
  }
  if (step.set && step.type) {
    await typeInto(step.set, step.value ?? '', signal);
    return;
  }
  if (step.set && step.select) {
    const el = (await waitForElement(step.set, signal)) as HTMLSelectElement | null;
    if (el) {
      el.value = step.value ?? '';
      el.dispatchEvent(new Event('change', { bubbles: true }));
    }
    return;
  }
  if (step.set && step.check !== undefined) {
    const el = await waitForElement(step.set, signal);
    el?.dispatchEvent(
      new CustomEvent('change', { detail: { checked: step.check }, bubbles: true }),
    );
    return;
  }
  if (step.set) {
    const el = await waitForElement(step.set, signal);
    el?.dispatchEvent(new CustomEvent('change', { detail: { value: step.value }, bubbles: true }));
  }
}

/**
 * Runs a drive script. Resolves when finished or silently on abort.
 */
export async function runDrive(steps: DriveStep[], signal: AbortSignal): Promise<void> {
  try {
    for (const step of steps) {
      if (signal.aborted) return;
      await runStep(step, signal);
      if (!step.wait) await sleep(STEP_MS, signal);
    }
  } catch (err) {
    if ((err as DOMException)?.name !== 'AbortError') {
      // eslint-disable-next-line no-console
      console.warn('[presentation/drive] step failed', err);
    }
  }
}
