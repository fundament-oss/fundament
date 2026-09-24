import {
  Component,
  computed,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { CATALOG } from '../../connect/tokens';
import {
  ConfigSchemaEntry,
  GetPluginDefinitionRequestSchema,
} from '../../generated/catalog/v1/catalog_pb';
import SheetSyncDirective from '../sheet-sync.directive';
import PluginConfigFormComponent from './plugin-config-form.component';
import {
  getInstallStatusDisplay,
  isInstallInProgress,
  isInstallFailed,
  isInstallRunning,
  isInstallTerminating,
} from '../utils/plugin-install-status';

import '@nldd/design-system/badge';
import '@nldd/design-system/button';
import '@nldd/design-system/cell';
import '@nldd/design-system/icon-cell';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/menu';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/top-title-bar';

interface Cluster {
  id: string;
  name: string;
  // null when the plugin is not installed on this cluster; otherwise the
  // PluginInstallation status phase (Pending, Deploying, Running, …).
  phase: string | null;
  // The version pinned on this cluster; empty when not installed. A plugin is
  // installed per cluster, so two clusters can run different versions.
  version: string;
  running: boolean;
}

// A published definition the user can pin on install: the version they see and
// the content hash that version resolves to.
export interface PluginVersionOption {
  version: string;
  hash: string;
}

// Emitted on install: the chosen clusters plus the pinned version/hash pair.
export interface InstallSelection {
  clusterIds: string[];
  version: string;
  hash: string;
  // Chosen config values; {} when the definition declares no schema or all
  // defaults were kept.
  config: Record<string, string>;
}

// Emitted on retry: a single cluster plus the currently pinned version/hash.
export interface RetrySelection {
  clusterId: string;
  version: string;
  hash: string;
}

/** The version shows from the moment a cluster has one until the removal is
 *  through: while it is being torn down the plugin is still installed at that
 *  version. Only a new install has nothing to report yet. */
function showsInstalledVersion(cluster: Cluster): boolean {
  if (!cluster.version || cluster.phase === null) return false;
  return !isInstallInProgress(cluster.phase) || isInstallTerminating(cluster.phase);
}

@Component({
  selector: 'app-install-plugin-modal',
  imports: [SheetSyncDirective, PluginConfigFormComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './install-plugin-modal.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class InstallPluginModalComponent {
  /** Human-readable plugin name (e.g. "OpenFSC"), shown to the user. Never the
   * install identifier — the caller keeps that to itself. */
  pluginDisplayName = input('');

  // The publisher and install identifiers, for GetPluginDefinition's PluginRef
  // lookup: neither the display name above nor the modal's own state carries
  // these, so the parent hands them in.
  organizationName = input('');

  pluginName = input('');

  clusters = input<Cluster[]>([]);

  // Published versions to choose from, latest first. Empty means nothing is
  // published yet — the plugin cannot be installed.
  versions = input<PluginVersionOption[]>([]);

  // True when fetching the versions failed (as opposed to succeeding with an
  // empty list). Lets the modal distinguish a transient error from "nothing
  // published yet" so it doesn't send the user chasing a publishing problem.
  versionsError = input(false);

  show = input(false);

  closeModal = output<void>();

  // Emits the chosen clusters plus the pinned version/hash to install.
  install = output<InstallSelection>();

  // Emits the cluster ID to uninstall the plugin from.
  uninstall = output<string>();

  // Emits a cluster to retry a failed installation on, with the current pin.
  retry = output<RetrySelection>();

  private catalogClient = inject(CATALOG);

  // Set once a version with a non-empty schema is chosen; the form renders
  // against it and clears it back to null on submit, cancel or close.
  pendingInstall = signal<{
    clusterId: string;
    option: PluginVersionOption;
    schema: ConfigSchemaEntry[];
  } | null>(null);

  schemaError = signal(false);

  schemaLoading = signal(false);

  // Keyed by `${organizationName}/${pluginName}/${version}`: GetPluginDefinition
  // is pinned per version AND per plugin, and a sheet stays open across several
  // "Install" clicks — including on a different plugin row — on the same
  // version list.
  private schemaCache = new Map<string, ConfigSchemaEntry[]>();

  // Bumped by every onInstallOne call and by onClose. A fetch that lands after
  // its generation has been superseded — a second click, or the sheet closing
  // — is stale: the component is never destroyed (both parents render it
  // unconditionally), so nothing else would stop a late response from
  // resurrecting state on a closed sheet or one reopened for another plugin.
  private requestGeneration = 0;

  /** Everything but the newest published version; the newest sits on its own at
   *  the top of the menu. */
  earlierVersions = computed(() => this.versions().slice(1));

  /** The published versions as a sentence fragment, newest first. */
  versionList = computed(() =>
    this.versions()
      .map((v) => v.version)
      .join(', '),
  );

  statusFor = getInstallStatusDisplay;

  isInProgress = isInstallInProgress;

  isFailed = isInstallFailed;

  isRunning = isInstallRunning;

  isRemoving = isInstallTerminating;

  showsVersion = showsInstalledVersion;

  onClose(): void {
    // A re-opened sheet starts clean: neither a pending form nor a stale error
    // nor a schema fetched for a version this session may never touch again
    // should survive across opens. Bumping the generation also drops any fetch
    // still in flight — this component is never destroyed, so without it a
    // late response could resurrect state after close.
    this.requestGeneration += 1;
    this.pendingInstall.set(null);
    this.schemaError.set(false);
    this.schemaLoading.set(false);
    this.schemaCache.clear();
    this.closeModal.emit();
  }

  /** One row, one install, at the version picked from that row's own menu. A
   *  plugin is pinned per cluster, so the version belongs to the row and not to
   *  the sheet. Fetches the version's config schema first: an empty schema
   *  installs immediately as before, a non-empty one opens the form. Guarded by
   *  a generation counter: the Install button is disabled while a fetch is in
   *  flight, but a second click (a different row) or a close can still race it,
   *  and a superseded result must not set state a later action already moved
   *  past. */
  async onInstallOne(clusterId: string, option: PluginVersionOption): Promise<void> {
    this.requestGeneration += 1;
    const generation = this.requestGeneration;
    this.schemaError.set(false);
    this.schemaLoading.set(true);
    let schema: ConfigSchemaEntry[];
    try {
      schema = await this.fetchConfigSchema(option.version, generation);
    } catch {
      // Without the schema there is no telling whether a form is needed, so
      // installing anyway could skip required config; surface the error instead.
      if (generation === this.requestGeneration) this.schemaError.set(true);
      return;
    } finally {
      if (generation === this.requestGeneration) this.schemaLoading.set(false);
    }
    if (generation !== this.requestGeneration) return;
    if (schema.length === 0) {
      // No declared config: instant install, exactly the pre-schema behavior.
      this.install.emit({ clusterIds: [clusterId], version: option.version, hash: option.hash, config: {} });
      return;
    }
    this.pendingInstall.set({ clusterId, option, schema });
  }

  private async fetchConfigSchema(version: string, generation: number): Promise<ConfigSchemaEntry[]> {
    // Computed before the await, alongside the generation: the org/plugin name
    // inputs could change while a fetch for a DIFFERENT plugin is in flight, so
    // the key must be pinned to what was asked, not read again after the await.
    const key = `${this.organizationName()}/${this.pluginName()}/${version}`;
    const cached = this.schemaCache.get(key);
    if (cached) return cached;
    const resp = await firstValueFrom(
      this.catalogClient.getPluginDefinition(
        create(GetPluginDefinitionRequestSchema, {
          lookup: {
            case: 'name',
            value: { organizationName: this.organizationName(), pluginName: this.pluginName() },
          },
          version,
        }),
      ),
    );
    if (resp.configSchemaUnavailable) {
      // The catalog could not parse the manifest, so it cannot say whether
      // required config exists — fail closed exactly like a fetch error:
      // caught by the caller's schemaError path, nothing cached.
      throw new Error('config schema unavailable');
    }
    // Only cache while still the current request: a fetch that resolves after
    // a second click or onClose() must not poison the cache for a later open.
    if (generation === this.requestGeneration) this.schemaCache.set(key, resp.configSchema);
    return resp.configSchema;
  }

  onConfigConfirmed(config: Record<string, string>): void {
    const pending = this.pendingInstall();
    if (!pending) return;
    this.pendingInstall.set(null);
    this.install.emit({
      clusterIds: [pending.clusterId],
      version: pending.option.version,
      hash: pending.option.hash,
      config,
    });
  }

  onConfigCancelled(): void {
    this.pendingInstall.set(null);
  }

  onUninstall(clusterId: string): void {
    this.uninstall.emit(clusterId);
  }

  /** Retries at the version already pinned on that cluster, falling back to the
   *  latest published one when the failed install never recorded a version. */
  onRetry(clusterId: string, pinned: string): void {
    const option = this.versions().find((v) => v.version === pinned) ?? this.versions()[0];
    if (!option) return;
    this.retry.emit({ clusterId, version: option.version, hash: option.hash });
  }
}
