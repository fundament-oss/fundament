import { Injectable, inject } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { AUTHZ } from '../connect/tokens';
import type { PermissionCheck } from '../generated/v1/authz_pb';

/** The most checks one CheckPermissions call accepts. */
const MAX_CHECKS = 500;

@Injectable({ providedIn: 'root' })
export default class PermissionsService {
  private client = inject(AUTHZ);

  /**
   * Whether the caller may take each action, in request order. Every answer is
   * false if the check fails; the failure is only logged.
   */
  async can(checks: PermissionCheck[]): Promise<boolean[]> {
    try {
      const answers: boolean[] = [];
      for (let i = 0; i < checks.length; i += MAX_CHECKS) {
        const chunk = checks.slice(i, i + MAX_CHECKS);
        // eslint-disable-next-line no-await-in-loop
        const res = await firstValueFrom(this.client.checkPermissions({ checks: chunk }));
        // A short reply would misalign answers with requests.
        if (res.allowed.length !== chunk.length) {
          throw new Error(`Expected ${chunk.length} answers, got ${res.allowed.length}`);
        }
        answers.push(...res.allowed);
      }
      return answers;
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('Permission check failed', err);
      return checks.map(() => false);
    }
  }
}
