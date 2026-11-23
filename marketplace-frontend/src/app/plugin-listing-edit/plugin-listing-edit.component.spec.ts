import { TestBed, type ComponentFixture } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap, provideRouter } from '@angular/router';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import type PluginListingEditComponent from './plugin-listing-edit.component';
import PluginDevelopmentService, {
  type PluginListing,
} from '../plugin-development/plugin-development.service';

const ORG_ID = '019b4000-0000-7000-8000-000000000001';

const listing: PluginListing = {
  id: 'p1',
  pendingReview: false,
  synced: {
    displayName: 'Cert Manager',
    descriptionShort: 'Automated TLS certificate management.',
    tags: ['certificates', 'tls'],
    repositoryUrl: 'https://github.com/cert-manager/cert-manager',
    license: 'Apache-2.0',
  },
  description: '## Overview',
  categoryIds: ['c1'],
  image: '',
  features: [{ id: 'f1', title: 'Renewal', body: 'Renews before expiry.' }],
  documentationLinks: [
    { id: 'l1', title: 'Docs', urlName: 'Official docs', url: 'https://cert-manager.io/docs' },
  ],
  visibility: 'public',
  allowedOrganizationIds: [],
  authorName: 'Fundament',
  authorUrl: 'https://example.com',
  logo: 'data:image/svg+xml;base64,PHN2Zy8+',
  logoDark: '',
  versionLogo: '',
  versionLogoDark: 'data:image/svg+xml;base64,ZGFyaw==',
};

function fileEvent(file?: File): Event {
  return new CustomEvent('change', { detail: { files: file ? [file] : [] } });
}

// Imported in beforeAll: the design system's components call matchMedia as
// they load, the file field observes its size, and jsdom's ElementInternals
// has no form API.
let EditComponent: typeof PluginListingEditComponent;

async function render(updateListing = vi.fn(async (value: PluginListing) => value)) {
  TestBed.configureTestingModule({
    providers: [
      provideRouter([]),
      {
        provide: ActivatedRoute,
        useValue: { snapshot: { paramMap: convertToParamMap({ id: 'p1' }) } },
      },
      {
        provide: PluginDevelopmentService,
        useValue: {
          getListing: async () => structuredClone(listing),
          listCategories: async () => [
            { id: 'c1', name: 'Security' },
            { id: 'c2', name: 'Networking' },
          ],
          updateListing,
        },
      },
    ],
  });
  const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
  const fixture: ComponentFixture<PluginListingEditComponent> =
    TestBed.createComponent(EditComponent);
  fixture.detectChanges();
  await fixture.whenStable();
  // ngOnInit awaits the listing and the categories: let both settle.
  await new Promise((resolve) => {
    setTimeout(resolve);
  });
  fixture.detectChanges();
  return { fixture, component: fixture.componentInstance, updateListing, navigate };
}

function inputEvent(value: string): Event {
  return { target: { value } } as unknown as Event;
}

// Called with `new`, which an arrow function cannot be.
function ResizeObserverStub() {
  return { observe: () => {}, unobserve: () => {}, disconnect: () => {} };
}

describe('PluginListingEditComponent', () => {
  beforeAll(async () => {
    vi.stubGlobal('matchMedia', () => ({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    }));
    vi.stubGlobal('ResizeObserver', ResizeObserverStub);
    ['setFormValue', 'setValidity'].forEach((method) => {
      if (!(method in ElementInternals.prototype)) {
        Object.defineProperty(ElementInternals.prototype, method, { value: () => {} });
      }
    });
    EditComponent = (await import('./plugin-listing-edit.component')).default;
  });

  it('shows the definition.yaml fields read-only', async () => {
    const { fixture } = await render();
    const root = fixture.nativeElement as HTMLElement;

    expect(root.textContent).toContain('Automated TLS certificate management.');
    expect(root.textContent).toContain('Apache-2.0');
    const values = Array.from(
      root.querySelectorAll('nldd-text-field, nldd-multi-line-text-field'),
    ).map((field) => (field as HTMLInputElement).value);
    expect(values).not.toContain('Cert Manager');
    expect(values).not.toContain('Automated TLS certificate management.');
  });

  it('saves the whole listing, including the fields left alone', async () => {
    const { component, updateListing, navigate } = await render();

    component.onDescriptionInput(inputEvent('## Rewritten'));
    await component.save();

    expect(updateListing).toHaveBeenCalledWith({
      ...listing,
      description: '## Rewritten',
    });
    expect(navigate).toHaveBeenCalledWith(['/manage', 'p1']);
  });

  it('saves an upstream container image, which is not a URL', async () => {
    const { component, updateListing } = await render();

    component.onImageInput(inputEvent('quay.io/jetstack/cert-manager-controller:v1.21.3'));
    await component.save();

    expect(updateListing).toHaveBeenCalledWith(
      expect.objectContaining({ image: 'quay.io/jetstack/cert-manager-controller:v1.21.3' }),
    );
  });

  it('says when saved edits are waiting for review', async () => {
    const { fixture } = await render();
    const note = () =>
      (fixture.nativeElement as HTMLElement).querySelector('[data-testid="review-note"]')
        ?.textContent ?? '';
    expect(note()).not.toContain('waiting for review');

    fixture.componentInstance.listing.update((current) =>
      current ? { ...current, pendingReview: true } : current,
    );
    fixture.detectChanges();
    expect(note()).toContain('waiting for review');
  });

  it('sends one allowed organization per non-blank line when restricted', async () => {
    const { component, updateListing } = await render();

    component.onVisibilityChange(new CustomEvent('change', { detail: { value: 'restricted' } }));
    component.onAllowedOrganizationsInput(inputEvent(`  ${ORG_ID}\n\n`));
    await component.save();

    expect(updateListing).toHaveBeenCalledWith(
      expect.objectContaining({ visibility: 'restricted', allowedOrganizationIds: [ORG_ID] }),
    );
  });

  it('does not save a relative URL or an organization that is not an ID', async () => {
    const { component, updateListing } = await render();

    component.onAuthorUrlInput(inputEvent('example.com'));
    component.onVisibilityChange(new CustomEvent('change', { detail: { value: 'restricted' } }));
    component.onAllowedOrganizationsInput(inputEvent('acme'));
    await component.save();

    expect(updateListing).not.toHaveBeenCalled();
    expect(component.error('authorUrl')).toContain('full URL');
    expect(component.error('allowedOrganizations')).toContain('"acme"');
  });

  it('uploads a logo, refuses one over the limit, and removes an upload', async () => {
    const { component, updateListing } = await render();
    const svg = '<svg xmlns="http://www.w3.org/2000/svg"/>';

    await component.onLogoChange(
      'logoDark',
      fileEvent(new File([svg], 'dark.svg', { type: 'image/svg+xml' })),
    );
    await component.onLogoChange(
      'logo',
      fileEvent(new File(['a'.repeat(16 * 1024 + 1)], 'big.svg', { type: 'image/svg+xml' })),
    );
    expect(component.logoErrors().logo).toContain('maximum is 16 KB');

    component.removeLogo('logo');
    await component.save();

    expect(updateListing).toHaveBeenCalledWith(
      expect.objectContaining({ logo: '', logoDark: `data:image/svg+xml;base64,${btoa(svg)}` }),
    );
  });

  it('restores the loaded upload when a pick is cleared', async () => {
    const { component } = await render();

    await component.onLogoChange(
      'logo',
      fileEvent(new File(['<svg/>'], 'other.svg', { type: 'image/svg+xml' })),
    );
    await component.onLogoChange('logo', fileEvent());

    expect(component.listing()?.logo).toBe(listing.logo);
  });

  it('adds and removes features and documentation links', async () => {
    const { component, updateListing } = await render();

    component.addFeature();
    component.onFeatureInput(1, 'title', inputEvent('Issuers'));
    component.removeFeature(0);
    component.removeLink(0);
    await component.save();

    expect(updateListing).toHaveBeenCalledWith(
      expect.objectContaining({
        features: [{ id: '', title: 'Issuers', body: '' }],
        documentationLinks: [],
      }),
    );
  });
});
