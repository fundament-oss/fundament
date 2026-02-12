import {
  Component,
  inject,
  OnInit,
  signal,
  ViewChild,
  ElementRef,
  ChangeDetectionStrategy,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import { create } from '@bufbuild/protobuf';
import { type Timestamp, timestampDate } from '@bufbuild/protobuf/wkt';
import { firstValueFrom } from 'rxjs';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerPlus,
  tablerTrash,
  tablerX,
  tablerCheck,
  tablerCopy,
  tablerBan,
  tablerAlertTriangle,
} from '@ng-icons/tabler-icons';
import ModalComponent from '../modal/modal.component';
import {
  type APIKey,
  ListAPIKeysRequestSchema,
  DeleteAPIKeyRequestSchema,
  CreateAPIKeyRequestSchema,
  RevokeAPIKeyRequestSchema,
} from '../../generated/v1/apikey_pb';
import { APIKEY } from '../../connect/tokens';
import { TitleService } from '../title.service';
import {
  formatDate as formatDateUtil,
  formatDateTime as formatDateTimeUtil,
} from '../utils/date-format';

const getNameError = (field?: { invalid: boolean | null; touched: boolean | null }): string => {
  if (field?.invalid && field?.touched) {
    return 'Name is required';
  }
  return '';
};

const formatDate = (timestamp: Timestamp | undefined): string => formatDateUtil(timestamp, 'Never');

const formatDateTime = (timestamp: Timestamp | undefined): string =>
  formatDateTimeUtil(timestamp, 'Never');

const isExpired = (timestamp: Timestamp | undefined): boolean => {
  if (!timestamp) return false;
  return timestampDate(timestamp) < new Date();
};

const isRevoked = (timestamp: Timestamp | undefined): boolean => timestamp !== undefined;

@Component({
  selector: 'app-api-keys',
  imports: [FormsModule, NgIcon, ModalComponent],
  viewProviders: [
    provideIcons({
      tablerPlus,
      tablerTrash,
      tablerX,
      tablerCheck,
      tablerCopy,
      tablerBan,
      tablerAlertTriangle,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './api-keys.component.html',
})
export default class ApiKeysComponent implements OnInit {
  @ViewChild('nameInput') nameInput?: ElementRef<HTMLInputElement>;

  private titleService = inject(TitleService);

  private apiKeyClient = inject(APIKEY);

  apiKeys = signal<APIKey[]>([]);

  loading = signal(false);

  error = signal<string | null>(null);

  // Creation form state
  isCreating = signal(false);

  newKeyName = signal('');

  newKeyExpiresInDays = signal<number | null>(null);

  // Modal state
  showRevokeModal = signal(false);

  showDeleteModal = signal(false);

  pendingKeyId = signal<string | null>(null);

  pendingKeyName = signal<string | null>(null);

  // Newly created token (only shown once)
  createdToken = signal<string | null>(null);

  createdTokenPrefix = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('API keys');
  }

  async ngOnInit() {
    await this.loadApiKeys();
  }

  async loadApiKeys() {
    this.loading.set(true);
    this.error.set(null);

    try {
      const request = create(ListAPIKeysRequestSchema, {});
      const response = await firstValueFrom(this.apiKeyClient.listAPIKeys(request));
      this.apiKeys.set(response.apiKeys);
    } catch (err) {
      this.error.set(
        err instanceof Error
          ? `Failed to load API keys: ${err.message}`
          : 'Failed to load API keys',
      );
    } finally {
      this.loading.set(false);
    }
  }

  openRevokeModal(apiKeyId: string, apiKeyName: string) {
    this.pendingKeyId.set(apiKeyId);
    this.pendingKeyName.set(apiKeyName);
    this.showRevokeModal.set(true);
  }

  openDeleteModal(apiKeyId: string, apiKeyName: string) {
    this.pendingKeyId.set(apiKeyId);
    this.pendingKeyName.set(apiKeyName);
    this.showDeleteModal.set(true);
  }

  async confirmRevoke() {
    const apiKeyId = this.pendingKeyId();
    if (!apiKeyId) return;

    this.showRevokeModal.set(false);
    this.loading.set(true);
    this.error.set(null);

    try {
      const request = create(RevokeAPIKeyRequestSchema, {
        apiKeyId,
      });
      await firstValueFrom(this.apiKeyClient.revokeAPIKey(request));

      // Reload the list after successful revocation
      await this.loadApiKeys();
    } catch (err) {
      this.error.set(
        err instanceof Error
          ? `Failed to revoke API key: ${err.message}`
          : 'Failed to revoke API key',
      );
      this.loading.set(false);
    }
  }

  async confirmDelete() {
    const apiKeyId = this.pendingKeyId();
    if (!apiKeyId) return;

    this.showDeleteModal.set(false);
    this.loading.set(true);
    this.error.set(null);

    try {
      const request = create(DeleteAPIKeyRequestSchema, {
        apiKeyId,
      });
      await firstValueFrom(this.apiKeyClient.deleteAPIKey(request));

      // Reload the list after successful deletion
      await this.loadApiKeys();
    } catch (err) {
      this.error.set(
        err instanceof Error
          ? `Failed to delete API key: ${err.message}`
          : 'Failed to delete API key',
      );
      this.loading.set(false);
    }
  }

  startCreating() {
    this.isCreating.set(true);
    this.newKeyName.set('');
    this.newKeyExpiresInDays.set(null);
    this.error.set(null);

    // Focus the name input field after Angular updates the view
    setTimeout(() => {
      this.nameInput?.nativeElement.focus();
    });
  }

  cancelCreating() {
    this.isCreating.set(false);
    this.newKeyName.set('');
    this.newKeyExpiresInDays.set(null);
  }

  async createApiKey() {
    const name = this.newKeyName().trim();
    if (!name) {
      return;
    }

    this.loading.set(true);
    this.error.set(null);

    try {
      const expiresInDays = this.newKeyExpiresInDays();
      const request = create(CreateAPIKeyRequestSchema, {
        name,
        ...(expiresInDays && expiresInDays > 0 ? { expiresInDays: BigInt(expiresInDays) } : {}),
      });

      const response = await firstValueFrom(this.apiKeyClient.createAPIKey(request));

      // Store the token to display to the user (only time it's shown)
      this.createdToken.set(response.token);
      this.createdTokenPrefix.set(response.tokenPrefix);

      // Reset the creation form
      this.isCreating.set(false);
      this.newKeyName.set('');
      this.newKeyExpiresInDays.set(null);

      // Reload the list to show the new key
      await this.loadApiKeys();
    } catch (err) {
      this.error.set(
        err instanceof Error
          ? `Failed to create API key: ${err.message}`
          : 'Failed to create API key',
      );
    } finally {
      this.loading.set(false);
    }
  }

  async copyToken() {
    const token = this.createdToken();
    if (!token) return;

    try {
      // Check if clipboard API is available (requires HTTPS or localhost)
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(token);
      } else {
        // Fallback: create a temporary textarea element
        const textarea = document.createElement('textarea');
        textarea.value = token;
        textarea.style.position = 'fixed';
        textarea.style.opacity = '0';
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand('copy');
        document.body.removeChild(textarea);
      }
    } catch {
      this.error.set('Failed to copy token to clipboard. Please copy it manually.');
    }
  }

  dismissToken() {
    this.createdToken.set(null);
    this.createdTokenPrefix.set(null);
  }

  getNameError = getNameError;

  formatDate = formatDate;

  formatDateTime = formatDateTime;

  isExpired = isExpired;

  isRevoked = isRevoked;
}
