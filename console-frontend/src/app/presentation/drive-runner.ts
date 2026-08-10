// Auto-drive runner for the walkthrough. Drives the live app pane through the same
// DOM events its components already handle (e.g. nldd-text-field's `input` CustomEvent
// with detail.value), so no component internals are touched.
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
  if (step.emit) {
    document.dispatchEvent(new CustomEvent(step.emit, { bubbles: true }));
    return;
  }
  if (step.click) {
    const el = await waitForElement(step.click, signal);
    // A design system control is a custom element wrapping the real button, and
    // clicking the wrapper leaves that button untouched: a popovertarget never
    // fires, so a menu that should open stays shut. Click the control inside.
    const inner = el?.shadowRoot?.querySelector<HTMLElement>('button, a');
    (inner ?? (el as HTMLElement | null))?.click();
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
export default async function runDrive(steps: DriveStep[], signal: AbortSignal): Promise<void> {
  try {
    // A while loop, not for..of (banned) and not an indexed for (prefer-for-of would
    // then ask for the banned form back).
    let i = 0;
    while (i < steps.length) {
      const step = steps[i];
      if (signal.aborted) return;
      await runStep(step, signal);
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
