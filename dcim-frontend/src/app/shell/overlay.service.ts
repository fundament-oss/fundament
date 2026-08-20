import { Injectable, signal } from '@angular/core';
import type { Asset, AssetCategory, CatalogEntry } from '../inventory/inventory';
import type { DatacenterInfo } from '../datacenters/datacenter.model';
import type { Rack } from '../racks/rack.model';
import type { TaskData } from '../task-management/task-api.service';
import type { Cable } from '../patch-mapping/cable.model';

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
  newAsset(): void {
    this.assetSheet.set({
      id: '',
      deviceCatalogId: '',
      assetTag: '',
      status: 'available',
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

  /** Open the cable form on a blank cable. The data center you were looking at
   *  comes along when there is one; the form asks for it either way. */
  newCable(dcId = ''): void {
    this.cableSheet.set({ dcId, status: 'connected' });
  }

  /** Open the cable form on one that exists. */
  editCable(cable: Cable): void {
    this.cableSheet.set({ ...cable });
  }

  closeCable(): void {
    this.cableSheet.set(null);
  }

  closeAll(): void {
    this.productSheet.set(null);
    this.datacenterSheet.set(null);
    this.rackSheet.set(null);
    this.taskSheet.set(null);
    this.assetSheet.set(null);
    this.cableSheet.set(null);
  }
}
