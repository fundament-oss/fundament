import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  ElementRef,
  inject,
  OnInit,
  signal,
  viewChild,
  TemplateRef,
  AfterViewInit,
  OnDestroy,
} from '@angular/core';
import { Title } from '@angular/platform-browser';
import { ActivatedRoute, Router } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { pageTitle } from '../../shell/page-title';
import { RackSlotType } from '../../../generated/v1/common_pb';
import { Asset, AssetStatus, CatalogEntry, HistoryEntry, NoteComment } from '../inventory';
import { ASSET_STATUSES, ASSET_STATUS_TAG_COLOR, ASSET_STATUS_LABEL } from '../asset-status';
import InventoryApiService from '../inventory-api.service';
import CatalogApiService from '../../catalog/catalog-api.service';
import NoteApiService from '../note-api.service';
import PlacementApiService, { RackOption } from '../placement-api.service';
import connectErrorMessage from '../../../connect/error';
import parseValidationError from '../../../connect/validation';
import categoryIcon from '../../shared/asset-category';
import { INVENTORY_PATH, inventoryViewTitle, isInventoryView } from '../inventory-views';
import InventoryNavComponent from '../inventory-nav';
import SecondaryNavService from '../../shell/secondary-nav.service';
import OverlayService from '../../shell/overlay.service';

@Component({
  selector: 'app-asset-detail',
  templateUrl: './asset-detail.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [InventoryNavComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  // No styling of its own: the page inside paints the surface and owns the
  // layout, and styles.css takes this element out of the flow (display:
  // contents) so it cannot come between the pane and the page.
})
export default class AssetDetailComponent implements OnInit, AfterViewInit, OnDestroy {
  private readonly secondaryNav = inject(SecondaryNavService);

  /** The asset form lives in the shell: one form, opened from three places. */
  private readonly overlays = inject(OverlayService);

  /** This section's menu, handed to the shell for as long as the page is open. */
  private readonly secondaryNavTemplate = viewChild.required<TemplateRef<unknown>>('secondaryNav');

  ngAfterViewInit(): void {
    this.secondaryNav.set(this.secondaryNavTemplate());
  }

  ngOnDestroy(): void {
    this.secondaryNav.clear(this.secondaryNavTemplate());
  }

  private readonly route = inject(ActivatedRoute);

  private readonly router = inject(Router);

  /**
   * Follows the catalog link inside the app while leaving the element a real
   * `<a href>`, so middle-click and "open in new tab" keep working. Only a plain
   * left click is intercepted; the modifiers mean "open this somewhere else".
   */
  protected openCatalogEntry(event: Event, id: string): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }

    event.preventDefault();
    this.router.navigate(['/catalog', id]);
  }

  private readonly title = inject(Title);

  private readonly inventoryApi = inject(InventoryApiService);

  private readonly catalogApi = inject(CatalogApiService);

  private readonly noteApi = inject(NoteApiService);

  private readonly placementApi = inject(PlacementApiService);

  readonly assetId = computed(() => this.route.snapshot.paramMap.get('id') ?? '');

  readonly asset = signal<Asset | undefined>(undefined);

  /** False until the API call settles, so "not found" only shows after loading. */
  readonly assetLoaded = signal(false);

  readonly catalogEntry = signal<CatalogEntry | undefined>(undefined);

  /** Resolved physical location; undefined until loaded, or when the asset is unplaced. */
  readonly assetLocation = signal<
    { datacenter: string; rack: string; rackUnit: number; slotType: RackSlotType } | undefined
  >(undefined);

  readonly assetHistory = signal<HistoryEntry[]>([]);

  // ── Edit asset ─────────────────────────────────────────────────────────────

  /** Holds the asset being edited; non-null while the edit sheet is open. */
  readonly editAsset = signal<Partial<Asset> | null>(null);

  /** Bound values for the edit form's <select>s (seeded on open, read on save). */
  readonly assetStatus = signal<AssetStatus>('available');

  readonly assetRackId = signal<string>('');

  /** The slot type as the enum the API takes; empty until one is picked. */
  readonly assetSlotType = signal<RackSlotType | ''>('');

  /** The place you picked for the rack list, empty until you touch it. */
  readonly pickedLocation = signal<string>('');

  readonly invalidFields = signal<Record<string, string>>({});

  readonly formErrorMessage = signal<string | null>(null);

  readonly statuses = ASSET_STATUSES;

  /** Rack placement of the asset being edited; null while loading or when unplaced. */
  readonly editPlacement = signal<{
    id: string;
    rackId: string;
    unit: number;
    slotType: RackSlotType;
  } | null>(null);

  /** All racks, for the location picker. */
  readonly racks = signal<RackOption[]>([]);

  /** Racks grouped by datacenter, for the location <select> optgroups. */
  readonly racksByDatacenter = computed(() => {
    const groups = new Map<string, RackOption[]>();
    this.racks().forEach((rack) => {
      const list = groups.get(rack.datacenter) ?? [];
      list.push(rack);
      groups.set(rack.datacenter, list);
    });
    return [...groups.entries()]
      .map(([datacenter, racks]) => ({ datacenter, racks }))
      .sort((a, b) => a.datacenter.localeCompare(b.datacenter));
  });

  /** The places that have racks, in the order the groups come out. */
  readonly locations = computed(() => this.racksByDatacenter().map((group) => group.datacenter));

  /**
   * The place the rack list is limited to. Falls back to the first one, so the
   * rack picker always has something to show: a control you cannot use until
   * you have used another one first reads as broken.
   */
  readonly rackLocation = computed(() => this.pickedLocation() || this.locations()[0] || '');

  /** Only the racks that stand at the place you picked. */
  readonly racksAtLocation = computed(
    () =>
      this.racksByDatacenter().find((group) => group.datacenter === this.rackLocation())?.racks ??
      [],
  );

  readonly slotTypes: { value: RackSlotType; label: string }[] = [
    { value: RackSlotType.UNIT, label: 'Unit' },
    { value: RackSlotType.POWER, label: 'Power' },
    { value: RackSlotType.ZERO_U, label: 'Zero-U' },
  ];

  readonly slotTypeLabel = (slotType: RackSlotType): string =>
    this.slotTypes.find((s) => s.value === slotType)?.label ?? '—';

  private readonly assetSheetEl = viewChild<ElementRef>('assetSheet');

  /** Set while the delete confirmation is open. */
  readonly confirmingDelete = signal(false);

  /** Set while the decommission confirmation is open. */
  readonly confirmingDecommission = signal(false);

  private readonly deleteModalEl = viewChild<ElementRef>('deleteModal');

  private readonly decommissionModalEl = viewChild<ElementRef>('decommissionModal');

  private readonly fAssetTag = viewChild<ElementRef>('fAssetTag');

  private readonly fAssetSerial = viewChild<ElementRef>('fAssetSerial');

  private readonly fAssetWarranty = viewChild<ElementRef>('fAssetWarranty');

  private readonly fAssetRackUnit = viewChild<ElementRef>('fAssetRackUnit');

  private readonly fAssetNotes = viewChild<ElementRef>('fAssetNotes');

  /**
   * The list this page was opened from, so the way back leads to it and says its
   * name. Read once, while the navigation that brought us here is still in
   * flight; a deep link has no previous page and falls back to everything.
   */
  private readonly cameFrom = ((): string => {
    const previous = this.router.getCurrentNavigation()?.previousNavigation?.finalUrl?.toString();
    return previous && isInventoryView(previous) ? previous : `${INVENTORY_PATH}/all`;
  })();

  readonly backText = inventoryViewTitle(this.cameFrom);

  constructor() {
    // The tab says which one you have open, not just which section.
    effect(() => {
      const name = this.asset()?.assetTag;
      if (name) this.title.setTitle(pageTitle(name));
    });
    effect(() => {
      const el = this.assetSheetEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.editAsset() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.deleteModalEl()?.nativeElement as { show?: () => void; hide?: () => void };
      if (this.confirmingDelete()) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const el = this.decommissionModalEl()?.nativeElement as {
        show?: () => void;
        hide?: () => void;
      };
      if (this.confirmingDecommission()) el?.show?.();
      else el?.hide?.();
    });
  }

  /** Back from this page is back to the list it was opened from. */
  goToInventory(event: Event): void {
    // The back event bubbles and composes, so the shell's split view hears it
    // too and would navigate to the section on its own, a step short of where
    // this button says it goes.
    event.stopPropagation();
    this.router.navigateByUrl(this.cameFrom);
  }

  /** Where a row sits in the track, so the line starts and stops in the right place. */
  historyPosition(index: number): 'first' | 'between' | 'last' | 'only' {
    const last = this.assetHistory().length - 1;
    if (last === 0) return 'only';
    if (index === 0) return 'first';
    return index === last ? 'last' : 'between';
  }

  openDecommissionAsset(): void {
    this.confirmingDecommission.set(true);
  }

  cancelDecommissionAsset(): void {
    this.confirmingDecommission.set(false);
  }

  /**
   * Out of service, not gone: the asset keeps its history and its place in the
   * lists, and setting the status back undoes it. It asks first anyway, because
   * it says a machine in a rack is done — but the question is a plain one, and
   * the dialog does not look like the delete dialog, where the way out is the
   * primary button.
   */
  confirmDecommissionAsset(): void {
    const asset = this.asset();
    if (!asset || asset.status === 'decommissioned') {
      this.confirmingDecommission.set(false);
      return;
    }
    const updated: Asset = { ...asset, status: 'decommissioned' };
    firstValueFrom(this.inventoryApi.updateAsset(updated))
      .then(() => {
        this.asset.set(updated);
        this.confirmingDecommission.set(false);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  openDeleteAsset(): void {
    this.confirmingDelete.set(true);
  }

  cancelDeleteAsset(): void {
    this.confirmingDelete.set(false);
  }

  /** The asset is gone, so the page that showed it is too: back to the list. */
  confirmDeleteAsset(): void {
    const id = this.assetId();
    if (!id) return;
    firstValueFrom(this.inventoryApi.deleteAsset(id))
      .then(() => {
        this.confirmingDelete.set(false);
        this.router.navigateByUrl(INVENTORY_PATH);
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  ngOnInit(): void {
    this.loadAsset();
    this.loadHistory();
    this.loadNotes();
    this.loadLocation();
    this.loadRackOptions();
  }

  private loadRackOptions(): void {
    this.placementApi
      .listRackOptions()
      .then((racks) => this.racks.set(racks))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  private loadAsset(): void {
    firstValueFrom(this.inventoryApi.getAsset(this.assetId()))
      .then((res) => {
        const protoAsset = res.asset;
        if (!protoAsset) return undefined;
        if (!protoAsset.deviceCatalogId) {
          this.asset.set(InventoryApiService.mapAsset(protoAsset, new Map()));
          return undefined;
        }
        // Resolve the catalog entry so model, category and the specs panel populate.
        return firstValueFrom(this.catalogApi.getCatalogEntry(protoAsset.deviceCatalogId))
          .then((catRes) =>
            catRes.entry ? CatalogApiService.mapCatalogEntry(catRes.entry) : undefined,
          )
          .catch(() => undefined)
          .then((entry) => {
            const catalog = new Map<string, CatalogEntry>();
            if (entry) {
              catalog.set(entry.id, entry);
              this.catalogEntry.set(entry);
            }
            this.asset.set(InventoryApiService.mapAsset(protoAsset, catalog));
          });
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)))
      .finally(() => this.assetLoaded.set(true));
  }

  private loadHistory(): void {
    firstValueFrom(this.inventoryApi.getAssetEvents(this.assetId()))
      // Newest first: what happened to this asset last is what you came to read,
      // and it saves scrolling to the bottom of a long life.
      .then((res) =>
        this.assetHistory.set(
          res.events.map(InventoryApiService.mapAssetEvent).sort((a, b) => a.daysAgo - b.daysAgo),
        ),
      )
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  private loadNotes(): void {
    firstValueFrom(this.noteApi.listNotesForAsset(this.assetId()))
      .then((res) => this.notes.set(res.notes.map(NoteApiService.mapNote)))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  private loadLocation(): void {
    firstValueFrom(this.inventoryApi.getAssetLocation(this.assetId()))
      .then((res) => {
        const loc = res.location;
        this.assetLocation.set(
          loc
            ? {
                datacenter: loc.siteName,
                rack: loc.rackName,
                rackUnit: loc.rackUnitStart,
                slotType: loc.rackSlotType,
              }
            : undefined,
        );
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  isFieldInvalid(field: string): boolean {
    return field in this.invalidFields();
  }

  fieldError(field: string): string {
    return this.invalidFields()[field] ?? '';
  }

  private clearErrors(): void {
    this.invalidFields.set({});
    this.formErrorMessage.set(null);
  }

  private handleError(err: unknown): void {
    const { fields, message } = parseValidationError(err);
    this.invalidFields.set(fields);
    this.formErrorMessage.set(message);
  }

  /** The form is the one the shell holds, so it is the same one the inventory
   *  list and the add button in the bar open. */
  openEditAsset(): void {
    const current = this.asset();
    if (current) this.overlays.editAsset(current);
  }

  readonly notes = signal<NoteComment[]>([]);

  readonly newNoteText = signal('');

  readonly statusLabel = (status: AssetStatus): string => ASSET_STATUS_LABEL[status];

  readonly statusTagColor = (status: AssetStatus): string => ASSET_STATUS_TAG_COLOR[status];

  readonly statusIcon = (status: AssetStatus): string => {
    const icons: Record<AssetStatus, string> = {
      deployed: 'check-mark-circle',
      available: 'check-mark-circle',
      'needs-repair': 'exclamation-triangle',
      decommissioned: 'slash-circle',
      'on-order': 'arrow-right',
      requested: 'clock-arrow-counter-clockwise',
    };
    return icons[status];
  };

  readonly statusIconColor = (status: AssetStatus): string => {
    const colors: Record<AssetStatus, string> = {
      deployed: 'text-teal-500 dark:text-teal-400',
      available: 'text-green-500 dark:text-green-400',
      'needs-repair': 'text-amber-500 dark:text-amber-400',
      decommissioned: 'text-slate-400 dark:text-gray-500',
      'on-order': 'text-blue-500 dark:text-blue-400',
      requested: 'text-purple-500 dark:text-purple-400',
    };
    return colors[status];
  };

  readonly statusIconBgClass = (status: AssetStatus): string => {
    const classes: Record<AssetStatus, string> = {
      deployed: 'bg-teal-50 dark:bg-teal-950',
      available: 'bg-green-50 dark:bg-green-950',
      'needs-repair': 'bg-amber-50 dark:bg-amber-950',
      decommissioned: 'bg-slate-100 dark:bg-gray-800',
      'on-order': 'bg-blue-50 dark:bg-blue-950',
      requested: 'bg-purple-50 dark:bg-purple-950',
    };
    return `flex h-14 w-14 items-center justify-center rounded-full ${classes[status]}`;
  };

  readonly formatDaysAgo = (daysAgo: number): string => {
    if (daysAgo === 0) return 'Today';
    if (daysAgo === 1) return 'Yesterday';
    if (daysAgo < 30) return `${daysAgo} days ago`;
    const months = Math.floor(daysAgo / 30);
    return months === 1 ? '1 month ago' : `${months} months ago`;
  };

  readonly historyIcon = (action: HistoryEntry['action']): string => {
    const icons: Record<HistoryEntry['action'], string> = {
      received: 'arrow-right',
      deployed: 'check-mark-circle',
      moved: 'arrow-up-arrow-down',
      'repair-sent': 'gear',
      'repair-received': 'gear',
      decommissioned: 'slash-circle',
      requested: 'clock-arrow-counter-clockwise',
      note: 'info-circle',
    };
    return icons[action];
  };

  readonly historyIconBg = (action: HistoryEntry['action']): string => {
    const classes: Record<HistoryEntry['action'], string> = {
      received: 'bg-sky-50 dark:bg-sky-950 text-sky-500 dark:text-sky-400',
      deployed: 'bg-teal-50 dark:bg-teal-950 text-teal-500 dark:text-teal-400',
      moved: 'bg-sky-50 dark:bg-sky-950 text-sky-500 dark:text-sky-400',
      'repair-sent': 'bg-amber-50 dark:bg-amber-950 text-amber-500 dark:text-amber-400',
      'repair-received': 'bg-amber-50 dark:bg-amber-950 text-amber-500 dark:text-amber-400',
      decommissioned: 'bg-slate-100 dark:bg-gray-800 text-slate-500 dark:text-gray-400',
      requested: 'bg-purple-50 dark:bg-purple-950 text-purple-500 dark:text-purple-400',
      note: 'bg-indigo-50 dark:bg-indigo-950 text-indigo-500 dark:text-indigo-400',
    };
    return classes[action];
  };

  readonly categoryIcon = categoryIcon;
}
