// The review-backoffice build. See variant.ts, which this file replaces.
import StaffSessionService from './admin-review/staff-session.service';

export type Variant = 'catalog' | 'registry' | 'admin';

export const VARIANT: Variant = 'admin';

// The backoffice's reviewers sign in against the staff IDP, not the console.
// Named only here, so the storefront and the portal do not carry its code, nor
// the dcim-authn-api client behind it (FUN-20).
export const STAFF_SESSION: typeof StaffSessionService | null = StaffSessionService;
