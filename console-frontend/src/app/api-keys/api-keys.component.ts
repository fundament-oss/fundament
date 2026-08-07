import {
  Component,
  inject,
  Input,
  Output,
  EventEmitter,
  OnInit,
  signal,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  viewChild,
  ElementRef,
} from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { type Timestamp, timestampDate } from '@bufbuild/protobuf/wkt';
import { firstValueFrom } from 'rxjs';
import { createIdempotencyRef, withIdempotency } from '../../connect/idempotency';
import DialogSyncDirective from '../dialog-sync.directive';
import SheetSyncDirective from '../sheet-sync.directive';
import focusFirstModalInput from '../modal-focus';
import {
  type APIKey,
  ListAPIKeysRequestSchema,
  DeleteAPIKeyRequestSchema,
  CreateAPIKeyRequestSchema,
  RevokeAPIKeyRequestSchema,
} from '../../generated/v1/apikey_pb';
import { APIKEY } from '../../connect/tokens';
import { ToastService } from '../toast.service';
import {
  formatDate as formatDateUtil,
  formatDateTime as formatDateTimeUtil,
} from '../utils/date-format';
import AutofocusDirective from '../autofocus.directive';

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
  imports: [DialogSyncDirective, AutofocusDirective, SheetSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './api-keys.component.html',
})
export default class ApiKeysComponent implements OnInit {
  /** Owned by the shell: the sheet opens over whatever page you were on, so
   *  that page is not unmounted and is still there when you close it. */
  @Input()
  set show(open: boolean) {
    const wasOpen = this.isOpen;
    this.isOpen = open;
    // The sheet stays mounted between visits, so a reopen has to fetch: a key
    // made or revoked elsewhere would otherwise be missing from the list.
    if (open && !wasOpen && this.apiKeys().length > 0) this.loadApiKeys();
  }

  get show(): boolean {
    return this.isOpen;
  }

  private isOpen = false;

  @Output() closed = new EventEmitter<void>();

  private toastService = inject(ToastService);

  private apiKeyClient = inject(APIKEY);

  private idempotency = createIdempotencyRef();

  apiKeys = signal<APIKey[]>([]);

  loading = signal(false);

  error = signal<string | null>(null);

  // Creation form state
  isCreating = signal(false);

  newKeyName = signal('');

  /** Set by the submit, not by leaving the field: an empty field you have not
   *  tried to send yet is not a mistake. */
  newKeySubmitted = signal(false);

  newKeyExpiresIn = signal('');

  // Modal state
  showRevokeModal = signal(false);

  showDeleteModal = signal(false);

  pendingKeyId = signal<string | null>(null);

  pendingKeyName = signal<string | null>(null);

  // Newly created token (only shown once)
  createdToken = signal<string | null>(null);

  createdTokenPrefix = signal<string | null>(null);

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
    this.newKeySubmitted.set(false);
    this.newKeyExpiresIn.set('');
    this.error.set(null);
  }

  cancelCreating() {
    this.isCreating.set(false);
    this.newKeyName.set('');
    this.newKeySubmitted.set(false);
    this.newKeyExpiresIn.set('');
  }

  async createApiKey(event?: Event) {
    event?.preventDefault();

    this.newKeySubmitted.set(true);

    const name = this.newKeyName().trim();
    if (!name) {
      return;
    }

    this.loading.set(true);
    this.error.set(null);

    try {
      const expiresIn = this.newKeyExpiresIn();
      const request = create(CreateAPIKeyRequestSchema, {
        name,
        expiresIn,
      });

      const response = await withIdempotency(
        (opts) => this.apiKeyClient.createAPIKey(request, opts),
        { signal: this.idempotency.reset() },
      );

      // Store the token to display to the user (only time it's shown)
      this.createdToken.set(response.token);
      this.createdTokenPrefix.set(response.tokenPrefix);

      // Reset the creation form
      this.isCreating.set(false);
      this.newKeyName.set('');
      this.newKeyExpiresIn.set('');

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
      this.toastService.success('API key copied to clipboard');
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

  revokeDialogRef = viewChild<ElementRef<HTMLElement>>('revokeDialog');

  onRevokeModalOpen(): void {
    const el = this.revokeDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }

  deleteDialogRef = viewChild<ElementRef<HTMLElement>>('deleteDialog');

  onDeleteModalOpen(): void {
    const el = this.deleteDialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }

  /** The prefix, when it was last used and when it expires: what the columns of
   *  the old table said, in the line under the name. */
  keyDetail(apiKey: APIKey): string {
    const lastUsed = apiKey.lastUsed
      ? `Last used ${this.formatDateTime(apiKey.lastUsed)}`
      : 'Never used';
    const expires = apiKey.expires ? `Expires ${this.formatDate(apiKey.expires)}` : 'No expiry';
    return [apiKey.tokenPrefix, lastUsed, expires].join(' · ');
  }

  /** Back to what was behind it. A direct link has nothing to go back to, so
   *  that lands on the app's own empty state. */
  onClose(): void {
    this.closed.emit();
  }
}
