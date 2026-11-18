import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap } from '@angular/router';
import { of } from 'rxjs';
import MarketplaceIndexComponent, {
  UNCATEGORIZED,
  browseCategories,
  isUncategorized,
} from './index.component';
import MarketplaceService, {
  type Category,
  type MarketplacePluginSummary,
} from './marketplace.service';
import { TitleService } from '../title.service';
import { ConfigService } from '../config.service';

const categories: Category[] = [
  { id: 'cat-db', name: 'Database' },
  { id: 'cat-identity', name: 'Identity' },
  { id: 'cat-net', name: 'Networking' },
];

function plugin(id: string, categoryIds: string[]): MarketplacePluginSummary {
  return {
    id,
    name: id,
    displayName: id,
    tagline: '',
    vendor: '',
    logo: '',
    logoDark: '',
    image: '',
    categoryIds,
    categoryName: '',
    tags: [],
    labels: [],
    addedAt: '',
  };
}

// The shape the PoC catalog had: two plugins filed under a category, two under
// none, and a category nothing is in.
const plugins = [
  plugin('cnpg', ['cat-db']),
  plugin('istio', ['cat-net']),
  plugin('haven', []),
  plugin('ftv', []),
];

async function build() {
  TestBed.configureTestingModule({
    providers: [
      {
        provide: MarketplaceService,
        useValue: {
          listPlugins: async () => plugins,
          listCategories: async () => categories,
        },
      },
      { provide: TitleService, useValue: { setTitle: () => {} } },
      {
        provide: ConfigService,
        useValue: { getConfig: () => ({}), hasBundledDeveloperArea: () => false },
      },
      {
        provide: ActivatedRoute,
        useValue: { queryParamMap: of(convertToParamMap({})) },
      },
    ],
  });
  const component = TestBed.createComponent(MarketplaceIndexComponent).componentInstance;
  await component.ngOnInit();
  return component;
}

describe('marketplace browse counts', () => {
  it('leaves out the categories no plugin is in', () => {
    expect(browseCategories(categories, plugins).map((c) => c.name)).toEqual([
      'Database',
      'Networking',
    ]);
  });

  it('treats a category that is no longer listed as no category', () => {
    const listed = new Set(['cat-db']);
    expect(isUncategorized(plugin('x', ['cat-deleted']), listed)).toBe(true);
    expect(isUncategorized(plugin('y', ['cat-db']), listed)).toBe(false);
  });

  it('adds up to "All plugins" once the uncategorized ones have an entry', async () => {
    const component = await build();

    const entries = [
      ...component.shownCategories().map((c) => c.id),
      ...(component.hasUncategorized() ? [UNCATEGORIZED] : []),
    ];
    const sum = entries.reduce((total, id) => total + component.categoryCount(id), 0);

    expect(component.categoryCount(UNCATEGORIZED)).toBe(2);
    expect(sum).toBe(component.searchMatches().length);
  });

  it('lists the plugins in no category under Uncategorized', async () => {
    const component = await build();

    component.selectCategory(UNCATEGORIZED);

    expect(component.filteredPlugins().map((p) => p.id)).toEqual(['haven', 'ftv']);
    expect(component.selectedCategoryName()).toBe('Uncategorized');
  });

  it('shows as many plugins in each entry as its count says', async () => {
    const component = await build();

    [...component.shownCategories().map((c) => c.id), UNCATEGORIZED].forEach((id) => {
      component.selectCategory(id);
      expect(component.filteredPlugins().length).toBe(component.categoryCount(id));
    });
  });
});
