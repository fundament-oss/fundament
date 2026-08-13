import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { firstValueFrom } from 'rxjs';
import type { AssetCategory, CatalogEntry } from '../../inventory/inventory';
import CatalogApiService from '../catalog-api.service';
import parseValidationError from '../../../connect/validation';
import categoryIcon, { CATEGORIES } from '../../shared/asset-category';
import OverlayService from '../../shell/overlay.service';

interface NativeElementRef {
  nativeElement: { value: string; show?: () => void; hide?: () => void };
}

type InvalidFields = Record<string, string>;

/**
 * The product form, for a new product and for an existing one.
 *
 * It lives in the shell rather than in the catalog, because a product is made
 * from more than one place: the list, a product page, and in time the toolbar.
 * The catalog page used to own it and the product page kept a second copy of
 * the same fields, which is how the two drifted apart.
 *
 * The text fields are read from the DOM on save rather than bound both ways:
 * the design system's fields are custom elements with their own value, and one
 * read at save time beats an event per keystroke.
 */
@Component({
  selector: 'app-product-sheet',
  templateUrl: './product-sheet.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class ProductSheetComponent {
  private readonly overlays = inject(OverlayService);

  private readonly catalogApi = inject(CatalogApiService);

  /** The product being edited, or null when the sheet is closed. */
  readonly entry = computed(() => this.overlays.productSheet());

  readonly categories = CATEGORIES;

  /** The same icon a category wears in the menu and in a list row. */
  readonly categoryIcon = categoryIcon;

  readonly category = signal<AssetCategory>('Server');

  readonly specRows = signal<{ key: string; value: string }[]>([]);

  readonly errorMessage = signal<string | null>(null);

  readonly invalidFields = signal<InvalidFields>({});

  private readonly sheetEl = viewChild<NativeElementRef>('sheet');

  private readonly fModel = viewChild<NativeElementRef>('fModel');

  private readonly fManufacturer = viewChild<NativeElementRef>('fManufacturer');

  private readonly fPartNumber = viewChild<NativeElementRef>('fPartNumber');

  /**
   * What the sheet is actually showing, kept in step with its own open and
   * close events rather than counted from the signal. Signal changes coalesce:
   * close the sheet with Escape and open it again before the effect flushes,
   * and the effect sees one run with the new record and no run for the null in
   * between. Counting transitions there leaves the sheet shut for good.
   */
  private isOpen = false;

  /** The record the fields were filled from, so opening a different one seeds
   *  the form and a re-render does not overwrite what is being typed. */
  private seeded: Partial<CatalogEntry> | null = null;

  constructor() {
    effect(() => {
      const entry = this.entry();
      const el = this.sheetEl()?.nativeElement;
      if (!el) return;
      if (entry) {
        if (entry !== this.seeded) {
          this.seeded = entry;
          this.category.set((entry.category as AssetCategory) ?? 'Server');
          const rows = Object.entries(entry.specs ?? {}).map(([key, value]) => ({ key, value }));
          this.specRows.set(rows.length > 0 ? rows : [{ key: '', value: '' }]);
          this.clearErrors();
          this.fillFields(entry);
        }
        if (!this.isOpen) el.show?.();
      } else {
        this.seeded = null;
        if (this.isOpen) el.hide?.();
      }
    });
  }

  /**
   * Writes the record into the fields. They keep their own value once someone
   * types in them, and an attribute binding that goes from empty to empty is no
   * change at all, so a second New product would open on the last thing typed.
   */
  private fillFields(entry: Partial<CatalogEntry>): void {
    const model = this.fModel()?.nativeElement;
    const manufacturer = this.fManufacturer()?.nativeElement;
    const partNumber = this.fPartNumber()?.nativeElement;
    if (model) model.value = entry.model ?? '';
    if (manufacturer) manufacturer.value = entry.manufacturer ?? '';
    if (partNumber) partNumber.value = entry.partNumber ?? '';
  }

  /** The sheet says when it is open and when it is gone; Escape and a click
   *  outside close it without passing the Cancel button. */
  onSheetOpen(): void {
    this.isOpen = true;
  }

  onSheetClose(): void {
    this.isOpen = false;
    this.overlays.productSheet.set(null);
  }

  close(): void {
    this.overlays.productSheet.set(null);
  }

  /** One category at a time: unpicking the current one leaves it as it was. */
  onCategoryToggle(category: AssetCategory, selected: boolean): void {
    if (selected) this.category.set(category);
  }

  addSpecRow(): void {
    this.specRows.update((rows) => [...rows, { key: '', value: '' }]);
  }

  removeSpecRow(index: number): void {
    this.specRows.update((rows) => rows.filter((_, i) => i !== index));
  }

  updateSpecKey(index: number, event: Event): void {
    const val = (event.target as HTMLInputElement).value;
    this.specRows.update((rows) => rows.map((r, i) => (i === index ? { ...r, key: val } : r)));
  }

  updateSpecVal(index: number, event: Event): void {
    const val = (event.target as HTMLInputElement).value;
    this.specRows.update((rows) => rows.map((r, i) => (i === index ? { ...r, value: val } : r)));
  }

  save(): void {
    const form = this.entry();
    if (!form) return;

    this.clearErrors();

    const specs: Record<string, string> = {};
    this.specRows().forEach((row) => {
      if (row.key.trim()) specs[row.key.trim()] = row.value;
    });

    const entry: CatalogEntry = {
      id: form.id || '',
      model: this.fModel()?.nativeElement.value ?? '',
      manufacturer: this.fManufacturer()?.nativeElement.value ?? '',
      partNumber: this.fPartNumber()?.nativeElement.value ?? form.partNumber ?? '',
      category: this.category(),
      specs,
    };

    // Whoever is showing catalog data reloads it: the sheet is not on the page
    // it changes, so it cannot patch a list it does not have.
    const written = entry.id
      ? firstValueFrom(this.catalogApi.updateCatalogEntry(entry))
      : firstValueFrom(this.catalogApi.createCatalogEntry(entry));

    written
      .then(() => {
        this.catalogApi.markChanged();
        this.close();
      })
      .catch((err) => {
        const { fields, message } = parseValidationError(err);
        this.invalidFields.set(fields);
        this.errorMessage.set(message);
      });
  }

  /** Returns true when the given proto field name has a validation error. */
  isFieldInvalid(field: string): boolean {
    return field in this.invalidFields();
  }

  /** Returns the validation message for a proto field, or '' when valid. */
  fieldError(field: string): string {
    return this.invalidFields()[field] ?? '';
  }

  private clearErrors(): void {
    this.invalidFields.set({});
    this.errorMessage.set(null);
  }
}
