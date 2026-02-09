import {
  Component,
  inject,
  OnInit,
  signal,
  ViewChild,
  ElementRef,
  ChangeDetectionStrategy,
} from '@angular/core';
import { CommonModule } from '@angular/common';
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
} from '@ng-icons/tabler-icons';
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

@Component({
  selector: 'app-api-keys',
  imports: [CommonModule, FormsModule, NgIcon],
  viewProviders: [
    provideIcons({
      tablerPlus,
      tablerTrash,
      tablerX,
      tablerCheck,
      tablerCopy,
      tablerBan,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './api-keys.component.html',
})
export class ApiKeysComponent implements OnInit {
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
      this.error.set(err instanceof Error ? err.message : 'Failed to load API keys');
      console.error('Error loading API keys:', err);
    } finally {
      this.loading.set(false);
    }
  }

  async revokeApiKey(apiKeyId: string) {
    if (!confirm('Are you sure you want to revoke this API key? It will no longer be usable.')) {
      return;
    }

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
      this.error.set(err instanceof Error ? err.message : 'Failed to revoke API key');
      console.error('Error revoking API key:', err);
      this.loading.set(false);
    }
  }

  async deleteApiKey(apiKeyId: string) {
    if (!confirm('Are you sure you want to delete this API key? This action cannot be undone.')) {
      return;
    }

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
      this.error.set(err instanceof Error ? err.message : 'Failed to delete API key');
      console.error('Error deleting API key:', err);
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
      this.error.set(err instanceof Error ? err.message : 'Failed to create API key');
      console.error('Error creating API key:', err);
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
    } catch (err) {
      console.error('Failed to copy token:', err);
      this.error.set('Failed to copy token to clipboard. Please copy it manually.');
    }
  }

  dismissToken() {
    this.createdToken.set(null);
    this.createdTokenPrefix.set(null);
  }

  getNameError(field?: { invalid: boolean | null; touched: boolean | null }): string {
    if (field?.invalid && field?.touched) {
      return 'Name is required';
    }
    return '';
  }

  formatDate(timestamp: Timestamp | undefined): string {
    return formatDateUtil(timestamp, 'Never');
  }

  formatDateTime(timestamp: Timestamp | undefined): string {
    return formatDateTimeUtil(timestamp, 'Never');
  }

  isExpired(timestamp: Timestamp | undefined): boolean {
    if (!timestamp) return false;
    return timestampDate(timestamp) < new Date();
  }

  isRevoked(timestamp: Timestamp | undefined): boolean {
    return timestamp !== undefined;
  }
}
