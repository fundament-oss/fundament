
import {
  Component,
  inject,
  OnInit,
  signal,
  computed,
  ChangeDetectionStrategy,
} from '@angular/core';
import { ReactiveFormsModule } from '@angular/forms';
import { firstValueFrom } from 'rxjs';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerInfoCircle } from '@ng-icons/tabler-icons';
import { AUTHN, ORGANIZATION } from '../../connect/tokens';
import type { User } from '../../generated/authn/v1/authn_pb';
import type { Organization } from '../../generated/v1/organization_pb';
import { TitleService } from '../title.service';

@Component({
  selector: 'app-profile',
  imports: [ReactiveFormsModule, NgIcon],
  viewProviders: [provideIcons({ tablerInfoCircle })],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './profile.component.html',
})
export default class ProfileComponent implements OnInit {
  private titleService = inject(TitleService);

  private authnClient = inject(AUTHN);

  private orgClient = inject(ORGANIZATION);

  userInfo = signal<User | undefined>(undefined);

  organizations = signal<Organization[]>([]);

  isLoading = signal(true);

  error = signal<string | null>(null);

  organizationNames = computed(() => {
    const orgs = this.organizations();
    if (orgs.length === 0) return '';
    return orgs.map((o) => o.name).join(', ');
  });

  constructor() {
    this.titleService.setTitle('Profile');
  }

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
}
