// The developer-portal build. See variant.ts, which this file replaces.
import { InjectionToken } from '@angular/core';
import type StaffSessionService from './admin-review/staff-session.service';

export type Variant = 'catalog' | 'registry' | 'admin';

export const VARIANT: Variant = 'registry';

// Only the backoffice has a staff session; see variant.admin.ts.
export const STAFF_SESSION = new InjectionToken<StaffSessionService | null>('staff-session', {
  providedIn: 'root',
  factory: () => null,
});
