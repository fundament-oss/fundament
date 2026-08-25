import { Injectable } from '@angular/core';

/** One thing that happened to a rack, as the page lists it. */
export interface RackEvent {
  user: string;
  daysAgo: number;
  description: string;
}

/**
 * What has happened to a rack.
 *
 * Nothing, in the real app: assets have events on the server and racks do not,
 * so there is no rack-event API to read. This is the seam where one would land,
 * and until then the page shows no history rather than an invented one. The
 * demo provides its own, the way it does for the config and for who you are.
 */
@Injectable({ providedIn: 'root' })
export default class RackHistoryService {
  /** A field rather than a method, so the demo can put its own function in its
   *  place without either of them having to touch `this`. */
  readonly historyFor: (rackId: string) => RackEvent[] = () => [];
}
