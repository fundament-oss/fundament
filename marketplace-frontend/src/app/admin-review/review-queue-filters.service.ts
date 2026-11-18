import { Injectable, signal } from '@angular/core';
import { type Params } from '@angular/router';

// The filters (?status=, ?q=) of the queue the reviewer last looked at. The
// submission page takes them along on its way back, so a reviewer working
// through ?status=pending returns to that view after each decision, not only
// through the browser's Back.
@Injectable({ providedIn: 'root' })
export default class ReviewQueueFiltersService {
  readonly queryParams = signal<Params>({});
}
