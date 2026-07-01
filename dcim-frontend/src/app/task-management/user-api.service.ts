import { Injectable, inject } from '@angular/core';
import type { User as ProtoUser } from '../../generated/v1/user_pb';
import { USER_CLIENT } from '../../connect/tokens';

/** A roster member shown in the assignee dropdown / avatars. */
export interface RosterUser {
  id: string;
  name: string;
  initials: string;
  color: string;
  available: boolean;
}

@Injectable({ providedIn: 'root' })
export default class UserApiService {
  private readonly client = inject(USER_CLIENT);

  // Fixed avatar palette so colors stay stable per user and don't collide
  // arbitrarily. Keyed by a deterministic hash of the user id.
  private static readonly PALETTE = [
    'bg-blue-600',
    'bg-emerald-600',
    'bg-amber-600',
    'bg-violet-600',
    'bg-rose-600',
    'bg-cyan-600',
    'bg-indigo-600',
    'bg-teal-600',
  ];

  /** Maps an API user onto the roster model used by the assignee picker. */
  static mapUser(u: ProtoUser): RosterUser {
    return {
      id: u.id,
      name: u.name,
      initials: UserApiService.initials(u.name),
      color: UserApiService.color(u.id),
      // The directory has no presence concept; everyone is selectable.
      available: true,
    };
  }

  /** Derives up to two uppercase initials from a name. */
  static initials(name: string): string {
    const parts = name.trim().split(/\s+/).filter(Boolean);
    if (parts.length === 0) return '?';
    return parts
      .slice(0, 2)
      .map((p) => p[0]!.toUpperCase())
      .join('');
  }

  /** Deterministically maps an id onto a fixed palette entry. */
  static color(id: string): string {
    let hash = 0;
    for (let i = 0; i < id.length; i += 1) {
      hash = (hash * 31 + id.charCodeAt(i)) % 1_000_000_007;
    }
    const idx = hash % UserApiService.PALETTE.length;
    return UserApiService.PALETTE[idx];
  }

  listUsers() {
    return this.client.listUsers({});
  }
}
