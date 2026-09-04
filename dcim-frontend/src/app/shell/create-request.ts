import { effect, inject, untracked } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router } from '@angular/router';
import { map } from 'rxjs';

/**
 * `?new=1` on a section's address means "open the create form".
 *
 * It is what lets a form be opened from outside the page that holds it: the
 * add menu in the top bar navigates to the section and leaves this behind, and
 * the page picks it up when it arrives. It also makes the open form a place
 * you can link to and go back to.
 *
 * Read as a signal rather than in ngOnInit, because sections whose list and
 * detail share one route reuse the component: coming from a detail page it
 * never starts again. The parameter is cleared once the form is open, so a
 * reload or a Back does not open it a second time.
 *
 * @param open   Opens the form. Called once per request.
 * @param ready  Optional: hold the request until the page can honour it, for a
 *               form that needs something loaded first (a data center, say).
 */
export default function openOnCreateRequest(
  open: () => void,
  ready: () => boolean = () => true,
): void {
  const route = inject(ActivatedRoute);
  const router = inject(Router);
  const requested = toSignal(route.queryParamMap.pipe(map((params) => params.has('new'))), {
    initialValue: route.snapshot.queryParamMap.has('new'),
  });

  effect(() => {
    if (!requested() || !ready()) return;
    untracked(() => {
      open();
      router.navigate([], { queryParams: {}, replaceUrl: true });
    });
  });
}
