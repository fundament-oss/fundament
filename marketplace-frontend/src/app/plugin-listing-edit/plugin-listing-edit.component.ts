import {
  Component,
  inject,
  signal,
  computed,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import PluginDevelopmentService, {
  type Category,
  type ListingFeature,
  type ListingLink,
  type ListingVisibility,
  type PluginListing,
} from '../plugin-development/plugin-development.service';
import connectErrorMessage from '../../connect/error';
import '@nldd/design-system/checkbox-field';
import '@nldd/design-system/file-field';
import '@nldd/design-system/form';
import '@nldd/design-system/form-actions';
import '@nldd/design-system/radio-button-field';
import '@nldd/design-system/radio-button-group';
import '@nldd/design-system/text-field';
import '@nldd/design-system/validation-list';

// Limits mirror registry.v1.UpdatePluginRequest's validation.
export const MAX_DESCRIPTION = 8000;
export const MAX_CATEGORIES = 10;
export const MAX_FEATURES = 20;
export const MAX_LINKS = 20;
export const MAX_AUTHOR_NAME = 255;
export const MAX_IMAGE = 512;
// pluginruntime.MaxLogoBytes; the server also checks the SVG is safe to show.
export const MAX_LOGO_BYTES = 16 * 1024;
const LOGO_TYPES = ['image/svg+xml', 'image/png'];

export type LogoVariant = 'logo' | 'logoDark';

function readDataUri(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error ?? new Error('Could not read the file'));
    reader.readAsDataURL(file);
  });
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function isAbsoluteUrl(value: string): boolean {
  try {
    return Boolean(new URL(value).protocol);
  } catch {
    return false;
  }
}

function inputValue(event: Event): string {
  return (event.target as HTMLInputElement).value;
}

// Edits the listing fields UpdatePlugin takes. The fields definition.yaml owns
// are shown read-only: approving a version is what changes them. Saved edits
// wait for review too, except visibility.
@Component({
  selector: 'app-plugin-listing-edit',
  imports: [RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './plugin-listing-edit.component.html',
})
export default class PluginListingEditComponent implements OnInit {
  private titleService = inject(TitleService);

  private toastService = inject(ToastService);

  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private service = inject(PluginDevelopmentService);

  listing = signal<PluginListing | null>(null);

  categories = signal<Category[]>([]);

  // One organization ID per line, kept as typed so a half-written line is not
  // reformatted under the cursor.
  allowedOrganizationsText = signal('');

  isLoading = signal(true);

  isSaving = signal(false);

  submitted = signal(false);

  errorMessage = signal<string | null>(null);

  // A rejected pick, shown straight away rather than on save.
  logoErrors = signal<Record<LogoVariant, string>>({ logo: '', logoDark: '' });

  // Uploads as loaded, so clearing a pick in the file field restores them.
  private loadedLogos: Record<LogoVariant, string> = { logo: '', logoDark: '' };

  readonly maxDescription = MAX_DESCRIPTION;

  readonly maxFeatures = MAX_FEATURES;

  readonly maxLinks = MAX_LINKS;

  allowedOrganizationIds = computed(() =>
    this.allowedOrganizationsText()
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line !== ''),
  );

  // Field name to message; empty when the listing can be saved.
  errors = computed<Record<string, string>>(() => {
    const listing = this.listing();
    if (!listing) return {};
    const errors: Record<string, string> = {};
    if (listing.description.length > MAX_DESCRIPTION) {
      errors['description'] = `At most ${MAX_DESCRIPTION} characters.`;
    }
    if (listing.categoryIds.length > MAX_CATEGORIES) {
      errors['categories'] = `Pick at most ${MAX_CATEGORIES} categories.`;
    }
    if (listing.authorName.length > MAX_AUTHOR_NAME) {
      errors['authorName'] = `At most ${MAX_AUTHOR_NAME} characters.`;
    }
    if (listing.authorUrl !== '' && !isAbsoluteUrl(listing.authorUrl)) {
      errors['authorUrl'] = 'Enter a full URL, starting with https://.';
    }
    if (listing.image.length > MAX_IMAGE) {
      errors['image'] = `At most ${MAX_IMAGE} characters.`;
    }
    listing.documentationLinks.forEach((link, index) => {
      if (!isAbsoluteUrl(link.url)) {
        errors[`link-${index}`] = 'Enter a full URL, starting with https://.';
      }
    });
    if (listing.visibility === 'restricted') {
      const invalid = this.allowedOrganizationIds().find((id) => !UUID.test(id));
      if (invalid) errors['allowedOrganizations'] = `"${invalid}" is not an organization ID.`;
    }
    return errors;
  });

  hasErrors = computed(() => Object.keys(this.errors()).length > 0);

  logoVariants = computed(() => {
    const listing = this.listing();
    if (!listing) return [];
    return [
      {
        key: 'logo' as LogoVariant,
        label: 'Light mode',
        uploaded: listing.logo,
        version: listing.versionLogo,
      },
      {
        key: 'logoDark' as LogoVariant,
        label: 'Dark mode',
        uploaded: listing.logoDark,
        version: listing.versionLogoDark,
      },
    ];
  });

  async ngOnInit() {
    const id = this.route.snapshot.paramMap.get('id');
    if (!id) {
      this.errorMessage.set('Plugin id is missing');
      this.isLoading.set(false);
      return;
    }

    try {
      const [listing, categories] = await Promise.all([
        this.service.getListing(id),
        this.service.listCategories(),
      ]);
      if (!listing) {
        this.errorMessage.set('Plugin not found');
        return;
      }
      this.load(listing);
      this.categories.set(categories);
      this.titleService.setTitle(`Edit ${listing.synced.displayName} — My plugins`);
    } catch (error) {
      this.errorMessage.set(connectErrorMessage(error));
    } finally {
      this.isLoading.set(false);
    }
  }

  private load(listing: PluginListing) {
    this.listing.set(listing);
    this.loadedLogos = { logo: listing.logo, logoDark: listing.logoDark };
    this.allowedOrganizationsText.set(listing.allowedOrganizationIds.join('\n'));
  }

  private patch(change: Partial<PluginListing>) {
    this.listing.update((listing) => (listing ? { ...listing, ...change } : listing));
  }

  error(field: string): string {
    return this.submitted() ? (this.errors()[field] ?? '') : '';
  }

  onDescriptionInput(event: Event) {
    this.patch({ description: inputValue(event) });
  }

  onAuthorNameInput(event: Event) {
    this.patch({ authorName: inputValue(event) });
  }

  onAuthorUrlInput(event: Event) {
    this.patch({ authorUrl: inputValue(event) });
  }

  onImageInput(event: Event) {
    this.patch({ image: inputValue(event) });
  }

  isCategorySelected(id: string): boolean {
    return this.listing()?.categoryIds.includes(id) ?? false;
  }

  onCategoryChange(id: string, event: Event) {
    const { checked } = (event as CustomEvent<{ checked: boolean }>).detail;
    const current = this.listing()?.categoryIds ?? [];
    this.patch({
      categoryIds: checked
        ? [...current.filter((existing) => existing !== id), id]
        : current.filter((existing) => existing !== id),
    });
  }

  onVisibilityChange(event: Event) {
    const { value } = (event as CustomEvent<{ value: string }>).detail;
    this.patch({ visibility: value as ListingVisibility });
  }

  onAllowedOrganizationsInput(event: Event) {
    this.allowedOrganizationsText.set(inputValue(event));
  }

  async onLogoChange(variant: LogoVariant, event: Event) {
    const [file] = (event as CustomEvent<{ files: File[] }>).detail.files;
    this.logoErrors.update((errors) => ({ ...errors, [variant]: '' }));
    if (!file) {
      this.patch({ [variant]: this.loadedLogos[variant] });
      return;
    }
    if (!LOGO_TYPES.includes(file.type)) {
      this.logoErrors.update((errors) => ({ ...errors, [variant]: 'Choose an SVG or PNG file.' }));
      return;
    }
    if (file.size > MAX_LOGO_BYTES) {
      this.logoErrors.update((errors) => ({
        ...errors,
        [variant]: `The file is ${Math.ceil(file.size / 1024)} KB; the maximum is ${MAX_LOGO_BYTES / 1024} KB.`,
      }));
      return;
    }
    this.patch({ [variant]: await readDataUri(file) });
  }

  removeLogo(variant: LogoVariant) {
    this.logoErrors.update((errors) => ({ ...errors, [variant]: '' }));
    this.patch({ [variant]: '' });
  }

  addFeature() {
    const features = this.listing()?.features ?? [];
    this.patch({ features: [...features, { id: '', title: '', body: '' }] });
  }

  removeFeature(index: number) {
    const features = this.listing()?.features ?? [];
    this.patch({ features: features.filter((_, i) => i !== index) });
  }

  onFeatureInput(index: number, field: keyof Omit<ListingFeature, 'id'>, event: Event) {
    const features = this.listing()?.features ?? [];
    this.patch({
      features: features.map((feature, i) =>
        i === index ? { ...feature, [field]: inputValue(event) } : feature,
      ),
    });
  }

  addLink() {
    const links = this.listing()?.documentationLinks ?? [];
    this.patch({ documentationLinks: [...links, { id: '', title: '', urlName: '', url: '' }] });
  }

  removeLink(index: number) {
    const links = this.listing()?.documentationLinks ?? [];
    this.patch({ documentationLinks: links.filter((_, i) => i !== index) });
  }

  onLinkInput(index: number, field: keyof Omit<ListingLink, 'id'>, event: Event) {
    const links = this.listing()?.documentationLinks ?? [];
    this.patch({
      documentationLinks: links.map((link, i) =>
        i === index ? { ...link, [field]: inputValue(event) } : link,
      ),
    });
  }

  async save() {
    this.submitted.set(true);
    const listing = this.listing();
    if (!listing || this.hasErrors()) return;

    this.isSaving.set(true);
    try {
      const saved = await this.service.updateListing({
        ...listing,
        allowedOrganizationIds: this.allowedOrganizationIds(),
      });
      if (saved) this.load(saved);
      this.toastService.success(
        saved?.pendingReview
          ? 'Listing saved. The changes wait for review with your next submitted version.'
          : 'Listing saved',
      );
      await this.router.navigate(['/manage', listing.id]);
    } catch (error) {
      this.toastService.error(connectErrorMessage(error));
    } finally {
      this.isSaving.set(false);
    }
  }
}
