import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  inject,
  input,
  OnInit,
  output,
} from '@angular/core';
import DatacenterListService from './datacenter-list.service';
import { statusTagColor } from './datacenter.model';

/**
 * The menu of this section: every data center, with how it is doing.
 *
 * It is a component of its own because two pages show it. The overview picks a
 * data center in place, and the page of one data center's rooms shows the same
 * menu beside it, the way a product page keeps the categories beside it. What a
 * click does is the page's business, so it comes out as an event.
 *
 * The list itself comes from the service that holds it, not from the page: a
 * step from one page to the next rebuilds this component, and an input would
 * arrive empty every time.
 */
@Component({
  selector: 'app-datacenter-nav',
  templateUrl: './datacenter-nav.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class DatacenterNavComponent implements OnInit {
  private readonly list = inject(DatacenterListService);

  readonly datacenters = this.list.datacenters;

  /** True while the list is still being fetched: an empty list is only empty
   *  once you know it is, and until then "no data centers" is a lie. */
  readonly loading = computed(() => !this.list.loaded());

  readonly selectedId = input<string>('');

  readonly dcSelected = output<string>();

  readonly add = output<void>();

  readonly statusTagColor = statusTagColor;

  ngOnInit(): void {
    this.list.load();
  }
}
