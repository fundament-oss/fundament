import {
  Component,
  inject,
  signal,
  computed,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { RouterLink, ActivatedRoute } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { TitleService } from '../title.service';
import MarketplaceService, { type MarketplacePlugin, type Category } from './marketplace.service';
import PluginCardComponent from './plugin-card.component';

@Component({
  selector: 'app-marketplace-home',
  imports: [RouterLink, PluginCardComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './marketplace-home.component.html',
})
export default class MarketplaceHomeComponent implements OnInit {
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

  private matchesFilters = (plugin: MarketplacePlugin): boolean => {
    if (this.selectedCategory() !== 'all' && plugin.category !== this.selectedCategory()) {
      return false;
    }
    const q = this.searchQuery().trim().toLowerCase();
    if (!q) return true;
    return (
      plugin.displayName.toLowerCase().includes(q) ||
      plugin.tagline.toLowerCase().includes(q) ||
      plugin.vendor.toLowerCase().includes(q) ||
      plugin.tags.some((tag) => tag.toLowerCase().includes(q))
    );
  };

  filteredPlugins = computed(() => this.plugins().filter(this.matchesFilters));

  featuredPlugins = computed(() => this.plugins().filter((plugin) => plugin.featured));

  recentlyAdded = computed(() =>
    [...this.plugins()].sort((a, b) => b.addedAt.localeCompare(a.addedAt)).slice(0, 6),
  );

  categoryCount(categoryId: string): number {
    return this.plugins().filter((plugin) => plugin.category === categoryId).length;
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
