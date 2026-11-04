import { inject, type Signal } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute } from '@angular/router';
import { map } from 'rxjs';

/** The route parameter `name`, following the route when its value changes; '' when absent. */
export default function routeParam(name: string): Signal<string> {
  const route = inject(ActivatedRoute);
  return toSignal(route.paramMap.pipe(map((params) => params.get(name) ?? '')), {
    initialValue: route.snapshot.paramMap.get(name) ?? '',
  });
}
