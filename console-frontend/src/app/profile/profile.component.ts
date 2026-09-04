import {
  Component,
  inject,
  Input,
  Output,
  EventEmitter,
  OnInit,
  signal,
  computed,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ReactiveFormsModule } from '@angular/forms';
import { firstValueFrom } from 'rxjs';
import { AUTHN, ORGANIZATION } from '../../connect/tokens';
import type { User } from '../../generated/authn/v1/authn_pb';
import type { Organization } from '../../generated/v1/organization_pb';
import SheetSyncDirective from '../sheet-sync.directive';

@Component({
  selector: 'app-profile',
  imports: [ReactiveFormsModule, SheetSyncDirective],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './profile.component.html',
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class ProfileComponent implements OnInit {
  /** Owned by the shell: the sheet opens over whatever page you were on, so
   *  that page is not unmounted and is still there when you close it. */
  @Input() show = false;

  @Output() closed = new EventEmitter<void>();

  private authnClient = inject(AUTHN);

  private orgClient = inject(ORGANIZATION);

  userInfo = signal<User | undefined>(undefined);

  organizations = signal<Organization[]>([]);

  isLoading = signal(true);

  error = signal<string | null>(null);

  organizationNames = computed(() => {
    const orgs = this.organizations();
    if (orgs.length === 0) return '';
    return orgs.map((o) => o.alias).join(', ');
  });

  async ngOnInit() {
    await this.loadUserInfo();
  }

  private async loadUserInfo() {
    try {
      const [userResponse, orgResponse] = await Promise.all([
        firstValueFrom(this.authnClient.getUserInfo({})),
        firstValueFrom(this.orgClient.listOrganizations({})),
      ]);
      this.userInfo.set(userResponse.user);
      this.organizations.set(orgResponse.organizations);
      this.isLoading.set(false);
    } catch (error) {
      this.error.set(
        error instanceof Error
          ? `Failed to load user information: ${error.message}`
          : 'Failed to load user information',
      );
      this.isLoading.set(false);
    }
  }

  /** Back to what was behind it. A direct link has nothing to go back to, so
   *  that lands on the app's own empty state. */
  onClose(): void {
    this.closed.emit();
  }
}
