import { ActivatedRouteSnapshot, Route } from '@angular/router';
import RemountOnParamChangeStrategy from './remount-on-param-change';

const pluginsRoute: Route = { path: 'clusters/:id/plugins', data: { remountOnParamChange: true } };
const projectsRoute: Route = { path: 'clusters/:id/projects' };

function snapshot(routeConfig: Route, params: Record<string, string>): ActivatedRouteSnapshot {
  return { routeConfig, data: routeConfig.data ?? {}, params } as unknown as ActivatedRouteSnapshot;
}

describe('RemountOnParamChangeStrategy', () => {
  const strategy = new RemountOnParamChangeStrategy();

  it('remounts a route that asks for it when one of its parameters changes', () => {
    expect(
      strategy.shouldReuseRoute(
        snapshot(pluginsRoute, { id: 'b' }),
        snapshot(pluginsRoute, { id: 'a' }),
      ),
    ).toBe(false);
  });

  it('keeps it while the parameters stay the same', () => {
    expect(
      strategy.shouldReuseRoute(
        snapshot(pluginsRoute, { id: 'a' }),
        snapshot(pluginsRoute, { id: 'a' }),
      ),
    ).toBe(true);
  });

  it('keeps reusing every other route, as the router does by default', () => {
    expect(
      strategy.shouldReuseRoute(
        snapshot(projectsRoute, { id: 'b' }),
        snapshot(projectsRoute, { id: 'a' }),
      ),
    ).toBe(true);
  });

  it('never reuses across routes', () => {
    expect(
      strategy.shouldReuseRoute(
        snapshot(pluginsRoute, { id: 'a' }),
        snapshot(projectsRoute, { id: 'a' }),
      ),
    ).toBe(false);
  });
});
