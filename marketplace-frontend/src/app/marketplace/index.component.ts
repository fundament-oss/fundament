import {
  Component,
  inject,
  signal,
  computed,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { TitleService } from '../title.service';
import MarketplaceService, { type MarketplacePlugin, type Category } from './marketplace.service';
import PluginCardComponent from './plugin-card.component';
import PluginLabelsComponent from './plugin-labels.component';
import { PluginIconComponent } from '../icons';

@Component({
  selector: 'app-marketplace-index',
  imports: [PluginCardComponent, PluginLabelsComponent, PluginIconComponent, RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './index.component.html',
})
export default class MarketplaceIndexComponent implements OnInit {
  private titleService = inject(TitleService);

  private service = inject(MarketplaceService);

  private route = inject(ActivatedRoute);

  plugins = signal<MarketplacePlugin[]>([]);

  categories = signal<Category[]>([]);

  isLoading = signal(true);

  // 'all' or a category id.
  selectedCategory = signal<string>('all');

  // Search query, driven by the ?q= URL param set from the top-nav search box.
  searchQuery = signal<string>('');

  constructor() {
    this.route.queryParamMap.pipe(takeUntilDestroyed()).subscribe((params) => {
      this.searchQuery.set(params.get('q') ?? '');
    });
  }

  // True when the user is actively filtering; hides the curated home sections
  // in favour of a flat results grid.
  isFiltering = computed(
    () => this.selectedCategory() !== 'all' || this.searchQuery().trim().length > 0,
  );

  private matchesSearch = (plugin: MarketplacePlugin): boolean => {
    const q = this.searchQuery().trim().toLowerCase();
    if (!q) return true;
    return (
      plugin.displayName.toLowerCase().includes(q) ||
      plugin.tagline.toLowerCase().includes(q) ||
      plugin.vendor.toLowerCase().includes(q) ||
      plugin.tags.some((tag) => tag.toLowerCase().includes(q))
    );
  };

  // The search is scoped to the selected category: category and query narrow
  // the list together rather than one replacing the other.
  private matchesFilters = (plugin: MarketplacePlugin): boolean => {
    if (this.selectedCategory() !== 'all' && plugin.category !== this.selectedCategory()) {
      return false;
    }
    return this.matchesSearch(plugin);
  };

  filteredPlugins = computed(() => this.plugins().filter(this.matchesFilters));

  // Search hits across every category, ignoring the sidebar selection. Drives
  // the browse counts and the "search all categories" escape hatch, so a scoped
  // search that comes up empty can still point at where the matches live.
  searchMatches = computed(() => this.plugins().filter(this.matchesSearch));

  featuredPlugins = computed(() => this.plugins().filter((plugin) => plugin.featured));

  // The single featured pick gets a larger spotlight treatment; the rest fill the grid.
  spotlightPlugin = computed(() => this.featuredPlugins()[0] ?? null);

  gridFeatured = computed(() => this.featuredPlugins().slice(1));

  // Three featured plugins shown as floating preview cards in the hero panel.
  heroPlugins = computed(() => this.featuredPlugins().slice(0, 3));

  recentlyAdded = computed(() =>
    [...this.plugins()].sort((a, b) => b.addedAt.localeCompare(a.addedAt)).slice(0, 6),
  );

  coreCount = computed(
    () => this.plugins().filter((plugin) => plugin.labels.includes('core')).length,
  );

  // Counts track the active search, so the sidebar shows where the hits are
  // rather than a static catalogue total that contradicts the visible results.
  categoryCount(categoryId: string): number {
    return this.searchMatches().filter((plugin) => plugin.category === categoryId).length;
  }

  // Hits for the current query outside the selected category. Only meaningful
  // when the scoped result set is empty, which is where it is offered.
  widenableMatchCount = computed(() =>
    this.selectedCategory() === 'all' || !this.searchQuery().trim()
      ? 0
      : this.searchMatches().length,
  );

  emptyStateMessage = computed(() => {
    const query = this.searchQuery().trim();
    const category = this.selectedCategory();
    if (!query) return `No plugins in ${category} yet.`;
    const outside = this.widenableMatchCount();
    if (outside > 0) {
      return `No plugins in ${category} match “${query}”, but ${outside} elsewhere ${outside === 1 ? 'does' : 'do'}.`;
    }
    return `No plugins match “${query}”. Try a different search term.`;
  });

  // Infra-flavoured icon per category, shown in the browse sidebar.
  private readonly categoryIcons: Record<string, string> = {
    Database: 'cylinder-split',
    Networking: 'centralized-network',
    Observability: 'chart-x-y-axis-line',
    Security: 'shield-check-mark',
  };

  categoryIcon(categoryId: string): string {
    return this.categoryIcons[categoryId] ?? 'puzzle-piece';
  }

  async ngOnInit() {
    this.titleService.setTitle();
    const [plugins, categories] = await Promise.all([
      this.service.listPlugins(),
      this.service.listCategories(),
    ]);
    this.plugins.set(plugins);
    this.categories.set(categories);
    this.isLoading.set(false);
  }

  selectCategory(categoryId: string) {
    this.selectedCategory.set(categoryId);
  }
}
