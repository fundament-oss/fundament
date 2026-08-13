import { Injectable, signal } from '@angular/core';
import type { AssetCategory, CatalogEntry } from '../inventory/inventory';

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

  closeAll(): void {
    this.productSheet.set(null);
  }
}
