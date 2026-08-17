import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  inject,
  OnInit,
  untracked,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router } from '@angular/router';
import { filter, map, startWith } from 'rxjs';
import DatacenterListService from '../datacenters/datacenter-list.service';
import RackListService, { RackListItem } from './rack-list.service';

/**
 * The menu of this section: the racks of one data center, with which data
 * center that is above them.
 *
 * It is a component of its own because two pages show it: the rack, and the
 * page of one device in that rack. Opening a device replaces the rack beside
 * the menu, not the menu itself, so it has to keep standing.
 *
 * It reads the address rather than taking inputs: which rack is current follows
 * from the URL, and both pages are at a different one.
 */
@Component({
  selector: 'app-rack-nav',
  templateUrl: './rack-nav.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class RackNavComponent implements OnInit {
  private readonly router = inject(Router);

  private readonly dcList = inject(DatacenterListService);

  private readonly rackList = inject(RackListService);

  readonly datacenters = this.dcList.datacenters;

  readonly racks = this.rackList.racks;

  readonly selectedDcId = this.rackList.selectedDcId;

  /** The address as it is now, so a row lights up on any page that shows this
   *  menu: /racks/<id> and /racks/device/<id> both name a rack. */
  private readonly url = toSignal(
    this.router.events.pipe(
      filter((e): e is NavigationEnd => e instanceof NavigationEnd),
      map((e) => e.urlAfterRedirects),
      startWith(this.router.url),
    ),
    { initialValue: this.router.url },
  );

  readonly currentRackId = computed(() => {
    const [path] = this.url().split('?');
    const rest = path.startsWith('/racks/') ? path.slice('/racks/'.length) : '';
    return rest.startsWith('device/') ? this.rackList.openRackId() : rest;
  });

  readonly currentDcName = computed(
    () => this.datacenters().find((dc) => dc.id === this.selectedDcId())?.name ?? '',
  );

  constructor() {
    // A rack always stands in a data center, so the menu opens in one: the
    // first, until a page or a click says otherwise.
    effect(() => {
      const dcs = this.datacenters();
      untracked(() => {
        if (!this.selectedDcId() && dcs.length > 0) this.selectedDcId.set(dcs[0].id);
      });
    });
    // And it lists the racks of that one.
    effect(() => {
      const dcId = this.selectedDcId();
      untracked(() => this.rackList.load(dcId));
    });
  }

  ngOnInit(): void {
    this.dcList.load();
  }

  /** The data center above the list: a radio group, so only the button that
   *  becomes selected has anything to say. */
  onDcToggle(id: string, selected: boolean): void {
    if (selected && id !== this.selectedDcId()) this.selectedDcId.set(id);
  }

  readonly rackUsedU = (rack: RackListItem): number => rack.usedU;

  selectRack(id: string): void {
    this.router.navigate(['/racks', id]);
  }

  /**
   * Adding a rack is a form on the rack page, so from a device this first goes
   * there. The query says the form is open, which makes it a place you can
   * link to rather than a state only a click can reach.
   */
  addRack(): void {
    // Onto the rack you have open rather than /racks: that address opens the
    // first rack of the list on arrival, which would drop the query with it.
    const rackId = this.currentRackId() || this.racks()[0]?.id;
    this.router.navigate(rackId ? ['/racks', rackId] : ['/racks'], {
      queryParams: { new: 1 },
    });
  }
}
