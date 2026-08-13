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
import { CATEGORIES } from '../../shared/asset-category';
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

  readonly category = signal<AssetCategory>('Server');

  readonly specRows = signal<{ key: string; value: string }[]>([]);

  readonly errorMessage = signal<string | null>(null);

  readonly invalidFields = signal<InvalidFields>({});

  private readonly sheetEl = viewChild<NativeElementRef>('sheet');

  private readonly fModel = viewChild<NativeElementRef>('fModel');

  private readonly fManufacturer = viewChild<NativeElementRef>('fManufacturer');

  private readonly fPartNumber = viewChild<NativeElementRef>('fPartNumber');

  /** Whether the sheet was open on the previous run, so opening seeds the form
   *  once instead of on every keystroke that touches a signal it reads. */
  private wasOpen = false;

  constructor() {
    effect(() => {
      const entry = this.entry();
      const el = this.sheetEl()?.nativeElement;
      if (entry && !this.wasOpen) {
        this.category.set((entry.category as AssetCategory) ?? 'Server');
        this.specRows.set(
          Object.entries(entry.specs ?? {}).map(([key, value]) => ({ key, value })),
        );
        if (this.specRows().length === 0) this.specRows.set([{ key: '', value: '' }]);
        this.clearErrors();
        el?.show?.();
      } else if (!entry && this.wasOpen) {
        el?.hide?.();
      }
      this.wasOpen = entry !== null;
    });
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
