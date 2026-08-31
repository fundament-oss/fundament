import { Injectable, signal } from '@angular/core';
import type { Asset, AssetCategory, AssetStatus, CatalogEntry } from '../inventory/inventory';
import type { DatacenterInfo } from '../datacenters/datacenter.model';
import type { Rack } from '../racks/rack.model';
import type { TaskData } from '../task-management/task-api.service';
import type { Cable } from '../patch-mapping/cable.model';

/** Where a placement form starts. Every field is a starting point the form
 *  still asks about, so all of them are optional. */
export interface PlacementDraft {
  dcId?: string;
  rackId?: string;
  rackUnitStart?: number;
}

/**
 * The sheets the shell owns rather than a page.
 *
 * A form for one record belongs with the page that holds the record: it reads
 * that page's data and writes it back. A form that makes a new one belongs to
 * nobody. It has to open from a list, from a detail page and from the toolbar,
 * and a page that unmounts on navigation cannot hold a sheet that outlives it.
 * Those live here: one signal each, set to open, cleared to close.
 *
 * A sheet in the shell also stands outside the app's own layout, which is where
 * an overlay belongs. The design system stops its scroll mode and layer offsets
 * at the overlay's edge, so this is about who can open the sheet, not about how
 * it is drawn.
 */
@Injectable({ providedIn: 'root' })
export default class OverlayService {
  /**
   * The product form. Null when closed. An entry without an id is a new
   * product, which is why this carries the record rather than its id: whoever
   * opens it already has it in hand, and a new one is blank anyway.
   */
  readonly productSheet = signal<Partial<CatalogEntry> | null>(null);

  /** Open the product form on a blank product. Adding one from inside a
   *  category starts in that category: the view you are in is the answer to a
   *  question the form would otherwise ask again. */
  newProduct(category?: AssetCategory | 'all'): void {
    this.productSheet.set({
      id: '',
      model: '',
      manufacturer: '',
      partNumber: '',
      category: category && category !== 'all' ? category : 'Server',
      specs: {},
    });
  }

  /** Open the product form on an existing product. */
  editProduct(entry: CatalogEntry): void {
    this.productSheet.set(entry);
  }

  /**
   * The data center form. Null when closed. A record without an id is a new
   * one, which is what the add button in the bar opens.
   */
  readonly datacenterSheet = signal<Partial<DatacenterInfo> | null>(null);

  /** Open the data center form on a blank one. */
  newDatacenter(): void {
    this.datacenterSheet.set({
      id: '',
      name: '',
      fullName: '',
      city: '',
      country: '',
      address: '',
      tier: 3,
      established: new Date().getFullYear(),
      status: 'operational',
      floorSqm: 0,
    });
  }

  /** Open the data center form on one that exists. */
  editDatacenter(dc: DatacenterInfo): void {
    this.datacenterSheet.set({ ...dc });
  }

  closeDatacenter(): void {
    this.datacenterSheet.set(null);
  }

  /** The rack form. Null when closed. */
  readonly rackSheet = signal<(Partial<Rack> & { rowId?: string }) | null>(null);

  /** Open the rack form on a blank one. The data center you were looking at
   *  comes along when there is one; the form asks for it either way. */
  newRack(dcId = ''): void {
    this.rackSheet.set({ id: '', name: '', dcId, rowId: '', totalU: 42, devices: [] });
  }

  /** Open the rack form on one that exists. */
  editRack(rack: Partial<Rack> & { rowId?: string }): void {
    this.rackSheet.set({ ...rack });
  }

  closeRack(): void {
    this.rackSheet.set(null);
  }

  /** The task form. Null when closed. */
  readonly taskSheet = signal<Partial<TaskData> | null>(null);

  /** Open the task form on a blank task. */
  newTask(): void {
    this.taskSheet.set({ id: '', title: '', status: 'To do', priority: 'None', tags: [] });
  }

  /** Open the task form on one that exists. */
  editTask(task: TaskData): void {
    this.taskSheet.set({ ...task });
  }

  closeTask(): void {
    this.taskSheet.set(null);
  }

  /** The asset form. Null when closed. */
  readonly assetSheet = signal<Partial<Asset> | null>(null);

  /** Open the asset form on a blank asset. */
  /**
   * Open the asset form on one that does not exist yet.
   *
   * The status comes from the list you were looking at: standing in Requested
   * and pressing add means you are recording a request, and a form that opens
   * on Available makes you correct it every time. Falls back to Available where
   * the list is not one status, which is the one an asset most often arrives in.
   */
  newAsset(status: AssetStatus = 'available'): void {
    this.assetSheet.set({
      id: '',
      deviceCatalogId: '',
      assetTag: '',
      status,
      notes: '',
    });
  }

  /** Open the asset form on one that exists. */
  editAsset(asset: Asset): void {
    this.assetSheet.set({ ...asset });
  }

  closeAsset(): void {
    this.assetSheet.set(null);
    this.cableSheet.set(null);
  }

  /** The cable form. Null when closed. */
  readonly cableSheet = signal<Partial<Cable> | null>(null);

  /**
   * Open the cable form on a blank cable.
   *
   * Whatever the view you came from already answers comes along: the data
   * center, and on the section's own views the status, the type, the colour or
   * the device at one end. The form asks for all of it either way, so this is a
   * starting point and not a decision. Connected is the fallback status,
   * because a cable you add by hand is usually one that is already there.
   */
  newCable(prefill: Partial<Cable> = {}): void {
    this.cableSheet.set({ dcId: '', status: 'connected', ...prefill });
  }

  /** Open the cable form on one that exists. */
  editCable(cable: Cable): void {
    this.cableSheet.set({ ...cable });
  }

  closeCable(): void {
    this.cableSheet.set(null);
  }

  /**
   * Putting an asset in a rack. Null when closed.
   *
   * Not a record of its own but a placement: which asset, which rack, which
   * unit. The rack and the unit are a starting point, not the answer — the form
   * asks for both, so the same sheet works from a free slot in a rack and from
   * the add button in the bar, where there is no rack to inherit.
   */
  readonly placementSheet = signal<PlacementDraft | null>(null);

  /** Open it on a blank placement. Whatever you were looking at comes along:
   *  from a rack that rack, from a free slot that unit as well. */
  newPlacement(draft: PlacementDraft = {}): void {
    this.placementSheet.set({ ...draft });
  }

  closePlacement(): void {
    this.placementSheet.set(null);
  }

  closeAll(): void {
    this.productSheet.set(null);
    this.datacenterSheet.set(null);
    this.rackSheet.set(null);
    this.taskSheet.set(null);
    this.assetSheet.set(null);
    this.cableSheet.set(null);
    this.placementSheet.set(null);
  }
}
