// The developer-portal build. See variant.ts, which this file replaces.
import type StaffSessionService from './admin-review/staff-session.service';

export type Variant = 'catalog' | 'registry' | 'admin';

export const VARIANT: Variant = 'registry';

// Only the backoffice has a staff session; see variant.admin.ts.
export const STAFF_SESSION: typeof StaffSessionService | null = null;
