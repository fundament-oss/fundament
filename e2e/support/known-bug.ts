import {
  AfterAll,
  AfterStep,
  After,
  Before,
  BeforeStep,
  Status,
  setDefinitionFunctionWrapper,
} from '@cucumber/cucumber';
import { ICustomWorld } from './world.ts';

// @known-bug marks a scenario that fails on a filed product bug, with the
// issue's URL in the scenario's description. Such a scenario still runs: a
// failing step reports it as skipped, with the issue and the error in the
// report, so the run stays green; once it passes, it fails and asks for the
// tag to go. Only its steps are covered, so a failing hook still fails it.

const issues = new WeakMap<object, string>();
const inStep = new WeakSet<object>();
const hit: string[] = [];

Before({ tags: '@known-bug' }, function (this: ICustomWorld, { gherkinDocument, pickle }) {
  const children = gherkinDocument.feature?.children ?? [];
  const scenario = children
    .flatMap((c) => [c.scenario, ...(c.rule?.children.map((r) => r.scenario) ?? [])])
    .find((s) => s && pickle.astNodeIds.includes(s.id));
  const url = scenario?.description.match(/https?:\/\/\S+/)?.[0];
  if (!url) throw new Error(`@known-bug needs the issue's URL in the description of "${pickle.name}"`);
  issues.set(this, url);
});

BeforeStep({ tags: '@known-bug' }, function (this: ICustomWorld) {
  inStep.add(this);
});

AfterStep({ tags: '@known-bug' }, function (this: ICustomWorld) {
  inStep.delete(this);
});

setDefinitionFunctionWrapper(function (fn: (...args: unknown[]) => unknown) {
  return function (this: ICustomWorld, ...args: unknown[]) {
    if (!inStep.has(this)) return fn.apply(this, args);
    const known = (err: unknown) => {
      const url = issues.get(this);
      this.log(`Fails on the known bug ${url}: ${err instanceof Error ? err.message : String(err)}`);
      hit.push(url ?? '');
      return 'skipped';
    };
    try {
      const out = fn.apply(this, args);
      return out instanceof Promise ? out.catch(known) : out;
    } catch (err) {
      return known(err);
    }
  };
});

After({ tags: '@known-bug' }, function (this: ICustomWorld, { pickle, result }) {
  if (result?.status === Status.PASSED) {
    throw new Error(
      `"${pickle.name}" passes, so ${issues.get(this)} looks fixed: ` +
        `remove @known-bug from it in ${pickle.uri} and close the issue`,
    );
  }
});

AfterAll(function () {
  if (hit.length > 0) console.log(`\n${hit.length} skipped on known bugs: ${[...new Set(hit)].join(' ')}`);
});
