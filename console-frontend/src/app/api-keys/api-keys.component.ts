import {
  Component,
  inject,
  Input,
  Output,
  EventEmitter,
  signal,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  viewChild,
  ElementRef,
  effect,
  untracked,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
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
import { NotificationService } from '../notification.service';
import AuthnApiService from '../authn-api.service';
import OrganizationContextService from '../organization-context.service';
import {
  formatDate as formatDateUtil,
  formatDateTime as formatDateTimeUtil,
} from '../utils/date-format';
import AutofocusDirective from '../autofocus.directive';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/badge';
import '@nldd/design-system/banner';
import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/cell';
import '@nldd/design-system/container';
import '@nldd/design-system/form';
import '@nldd/design-system/form-actions';
import '@nldd/design-system/form-field';
import '@nldd/design-system/icon-button';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/menu';
import '@nldd/design-system/modal-dialog';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/text-field';
import '@nldd/design-system/title';
import '@nldd/design-system/toolbar';
import '@nldd/design-system/top-title-bar';
import '@nldd/design-system/validation-list';

/** The server's reason for refusing the lifetime, worded for under the field;
 *  null when the refusal is about something else. */
const getExpiresInRefusal = (err: unknown): string | null => {
  if (!(err instanceof ConnectError) || err.code !== Code.InvalidArgument) return null;
  const match = /expires_in: ([^\n[]+)/.exec(err.rawMessage);
  if (!match) return null;
  const reason = match[1].trim();
  return `${reason.charAt(0).toUpperCase()}${reason.slice(1)}.`;
};

/** What went wrong, without the `[code]` prefix Connect puts in front of its message. */
const errorText = (err: unknown): string => {
  if (err instanceof ConnectError) return err.rawMessage;
  return err instanceof Error ? err.message : 'Unknown error';
};

/** A call that was cut off rather than refused: only the component going away does that. */
const isAborted = (err: unknown): boolean =>
  (err instanceof ConnectError && err.code === Code.Canceled) ||
  (err instanceof DOMException && err.name === 'AbortError');

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
export default class ApiKeysComponent {
  /** Owned by the shell: the sheet opens over whatever page you were on, so
   *  that page is not unmounted and is still there when you close it. */
  @Input()
  set show(open: boolean) {
    const wasOpen = this.isOpen;
    this.isOpen = open;
    // The sheet stays mounted between visits, so a reopen has to fetch: a key
    // made or revoked elsewhere would otherwise be missing from the list. An
    // empty cache is the case that needs it most — no keys yet, or a first load
    // that failed — so the reopen refetches whatever the list currently holds.
    //
    // Only once the owner is known, though: the list is the organization's, and
    // a call made before the organization has been resolved carries no
    // organization header and is refused. The very first fetch is left to the
    // effect below, which runs as soon as both halves of the owner are in.
    if (open && !wasOpen && this.owner !== undefined) this.loadApiKeys();
    // Closing is the end of the visit: what it left behind, a half-filled form,
    // an error or the token, would otherwise greet the next one.
    if (!open && wasOpen) {
      this.visit += 1;
      this.resetTransientState();
    }
  }

  get show(): boolean {
    return this.isOpen;
  }

  private isOpen = false;

  /** Counts closes, so a call that outlives its visit knows it did. */
  private visit = 0;

  @Output() closed = new EventEmitter<void>();

  private notificationService = inject(NotificationService);

  private apiKeyClient = inject(APIKEY);

  private idempotency = createIdempotencyRef();

  apiKeys = signal<APIKey[]>([]);

  /** Starts true: the sheet is built for an open, and the first fetch may have
   *  to wait for the organization, so the list is pending from the first paint
   *  rather than briefly reading as "no keys yet". */
  loading = signal(true);

  error = signal<string | null>(null);

  // Creation form state
  isCreating = signal(false);

  newKeyName = signal('');

  /** The server's refusal of the name; the rules the field can check itself are
   *  in its validation list. */
  newKeyNameError = signal<string | null>(null);

  newKeyExpiresIn = signal('');

  newKeyExpiresInError = signal<string | null>(null);

  /** Longest lifetime the server allows, such as "8760h"; empty when keys may never expire. */
  maxExpiresIn = signal('');

  // Modal state
  showRevokeModal = signal(false);

  showDeleteModal = signal(false);

  pendingKeyId = signal<string | null>(null);

  pendingKeyName = signal<string | null>(null);

  /** A create is out. Not `loading`, which the list fetch shares: a submit made
   *  while a reopen refetches would otherwise be dropped. */
  creating = signal(false);

  // Newly created token (only shown once)
  createdToken = signal<string | null>(null);

  createdTokenPrefix = signal<string | null>(null);

  /** The key the token belongs to, so the token goes when the key does. */
  private createdKeyId: string | null = null;

  /** Counts list fetches, so only the latest one's answer is used. */
  private listRequest = 0;

  /** The latest list fetch is still out, so `loading` is its to clear. */
  private listPending = false;

  /** The fetch count when the token arrived. A list asked for before that may
   *  not hold the new key yet, so only a later one may say the key is gone. */
  private createdKeyListRequest = 0;

  private currentUser = toSignal(inject(AuthnApiService).currentUser$);

  private organizationContext = inject(OrganizationContextService);

  /** Whose keys this sheet holds: the user and the organization they were for. */
  private owner: string | undefined;

  constructor() {
    // The sheet stays mounted for the whole session, through a logout and the
    // next login. A token created by one user must not greet the next, and
    // neither may their list of keys, so a change of either drops it all.
    //
    // Only once both halves are known, though. Opening /<org>/api-keys directly
    // builds the sheet before the organization has been resolved, so the first
    // owner would read as "no organization" and the id arriving would look like
    // a change — emptying the list the sheet had just fetched.
    effect(() => {
      const owner = this.currentOwner();
      if (owner === undefined) return;

      untracked(() => {
        if (this.owner === undefined || this.owner === owner) {
          const firstOwner = this.owner === undefined;
          this.owner = owner;
          // Opening /<org>/api-keys directly builds the sheet before the
          // organization has been resolved, so the open it was built for could
          // not fetch yet. This is the moment it can.
          if (firstOwner && this.isOpen) this.loadApiKeys();
          return;
        }
        this.owner = owner;
        this.resetState();
        // Reopening refetches, but a sheet that is open right now stays open and
        // would go on showing the empty state it was just left with.
        if (this.isOpen) this.loadApiKeys();
      });
    });
  }

  /** Whose keys they are right now: unknown while either half is, as after a
   *  logout. `owner` keeps the last known one through that gap. */
  private currentOwner(): string | undefined {
    const userId = this.currentUser()?.id;
    const organizationId = this.organizationContext.currentOrganizationId();
    return userId && organizationId ? `${userId}/${organizationId}` : undefined;
  }

  /** Everything a visit leaves behind. The list stays: a reopen refetches it. */
  private resetTransientState() {
    this.dismissToken();
    this.cancelCreating();
    this.error.set(null);
    this.showRevokeModal.set(false);
    this.showDeleteModal.set(false);
    this.pendingKeyId.set(null);
    this.pendingKeyName.set(null);
  }

  private resetState() {
    this.resetTransientState();
    this.apiKeys.set([]);
    // A fetch still out is for the previous owner. With the sheet closed no
    // newer one passes it, so it is dropped here.
    this.listRequest += 1;
    this.listPending = false;
  }

  async loadApiKeys() {
    this.listRequest += 1;
    const { listRequest } = this;
    this.listPending = true;
    this.loading.set(true);
    this.error.set(null);

    try {
      const request = create(ListAPIKeysRequestSchema, {});
      const response = await firstValueFrom(this.apiKeyClient.listAPIKeys(request));
      // A newer fetch has started since: its answer is the one to show.
      if (listRequest !== this.listRequest) return;
      this.apiKeys.set(response.apiKeys);
      this.maxExpiresIn.set(response.maxExpiresIn);
      this.dismissTokenOfGoneKey(response.apiKeys, listRequest);
    } catch (err) {
      if (listRequest !== this.listRequest) return;
      this.error.set(`Failed to load API keys: ${errorText(err)}`);
    } finally {
      if (listRequest === this.listRequest) {
        this.listPending = false;
        this.loading.set(false);
      }
    }
  }

  /** A token whose key was deleted or revoked, here or elsewhere, is no use any
   *  more, and leaving it up says the key is still there. */
  private dismissTokenOfGoneKey(apiKeys: APIKey[], listRequest: number) {
    if (this.createdKeyId === null || listRequest <= this.createdKeyListRequest) return;
    const key = apiKeys.find((apiKey) => apiKey.id === this.createdKeyId);
    if (!key || isRevoked(key.revoked)) this.dismissToken();
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
      if (apiKeyId === this.createdKeyId) this.dismissToken();

      // Reload the list after successful revocation
      await this.loadApiKeys();
    } catch (err) {
      this.error.set(`Failed to revoke API key: ${errorText(err)}`);
      // A list fetch started meanwhile, by a reopen, still holds the list.
      if (!this.listPending) this.loading.set(false);
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
      if (apiKeyId === this.createdKeyId) this.dismissToken();

      // Reload the list after successful deletion
      await this.loadApiKeys();
    } catch (err) {
      this.error.set(`Failed to delete API key: ${errorText(err)}`);
      if (!this.listPending) this.loading.set(false);
    }
  }

  startCreating() {
    this.isCreating.set(true);
    this.newKeyName.set('');
    this.newKeyNameError.set(null);
    this.newKeyExpiresIn.set('');
    this.newKeyExpiresInError.set(null);
    this.error.set(null);
  }

  cancelCreating() {
    this.isCreating.set(false);
    this.newKeyName.set('');
    this.newKeyNameError.set(null);
    this.newKeyExpiresIn.set('');
    this.newKeyExpiresInError.set(null);
    this.error.set(null);
  }

  /** The form's only trigger: Enter in a field and the button both submit it,
   *  and the browser holds the submit back while a field's own rules fail. */
  async createApiKey(event: Event) {
    event.preventDefault();
    if (this.creating()) return;

    // A logout or another organization while the call is out: its token is not
    // for whoever is signed in by then. The live owner, not `owner`, which a
    // logout leaves standing.
    const owner = this.currentOwner();
    const { visit } = this;
    this.creating.set(true);
    this.error.set(null);
    this.newKeyNameError.set(null);
    this.newKeyExpiresInError.set(null);

    const request = create(CreateAPIKeyRequestSchema, {
      name: this.newKeyName().trim(),
      expiresIn: this.newKeyExpiresIn(),
    });

    try {
      const response = await withIdempotency(
        (opts) => this.apiKeyClient.createAPIKey(request, opts),
        { signal: this.idempotency.reset() },
      );
      if (this.currentOwner() !== owner) return;

      // A sheet closed while the call was out still takes the token: it was
      // never shown, and dropping it would leave a key nobody can use. The next
      // open shows it.

      // Store the token to display to the user (only time it's shown)
      this.createdToken.set(response.token);
      this.createdTokenPrefix.set(response.tokenPrefix);
      this.createdKeyId = response.id;
      this.createdKeyListRequest = this.listRequest;

      // Reset the creation form
      this.isCreating.set(false);
      this.newKeyName.set('');
      this.newKeyExpiresIn.set('');

      // Reload the list to show the new key
      await this.loadApiKeys();
    } catch (err) {
      // A refusal, though, is about a form the close already threw away.
      if (this.currentOwner() === owner && this.visit === visit) this.showCreateError(err, request);
    } finally {
      this.creating.set(false);
    }
  }

  /** Puts a refusal under the field it is about, and anything else in the banner.
   *  The fields stay editable while the call is out, so a refusal of a value
   *  that has been changed since is about nothing on screen, and goes. */
  private showCreateError(err: unknown, submitted: { name: string; expiresIn: string }) {
    if (isAborted(err)) return;
    if (err instanceof ConnectError && err.code === Code.AlreadyExists) {
      // Unique across the organization, while the list shows only your own
      // keys: the name may be taken by a key you cannot see.
      if (this.newKeyName().trim() !== submitted.name) return;
      this.newKeyNameError.set(
        'An API key with this name already exists in this organization, possibly made by another member.',
      );
      return;
    }
    const expiresInRefusal = getExpiresInRefusal(err);
    if (expiresInRefusal) {
      if (this.newKeyExpiresIn() === submitted.expiresIn) {
        this.newKeyExpiresInError.set(expiresInRefusal);
      }
      return;
    }
    this.error.set(`Failed to create API key: ${errorText(err)}`);
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
      this.notificationService.success('Token copied to clipboard');
    } catch {
      this.error.set('Failed to copy token to clipboard. Please copy it manually.');
    }
  }

  dismissToken() {
    this.createdToken.set(null);
    this.createdTokenPrefix.set(null);
    this.createdKeyId = null;
  }

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
