import { ActivatedRouteSnapshot, BaseRouteReuseStrategy } from '@angular/router';

/**
 * The router keeps a page's component when only its route parameters change,
 * from /clusters/A/plugins to /clusters/B/plugins say. A page that reads its
 * parameters once, as it opens, then goes on showing (and acting on) cluster
 * A. A route with `data: { remountOnParamChange: true }` gets a new component
 * instead, which reads the new parameters as it opens.
 */
export default class RemountOnParamChangeStrategy extends BaseRouteReuseStrategy {
  override shouldReuseRoute(future: ActivatedRouteSnapshot, curr: ActivatedRouteSnapshot): boolean {
    if (!super.shouldReuseRoute(future, curr)) return false;
    if (!future.data['remountOnParamChange']) return true;
    const keys = new Set([...Object.keys(future.params), ...Object.keys(curr.params)]);
    return [...keys].every((key) => future.params[key] === curr.params[key]);
  }
}
