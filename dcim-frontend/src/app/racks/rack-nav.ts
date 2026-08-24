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

  readonly racks = this.rackList.visible;

  /** Which building a rack stands in, said on the row. Rack names start again
   *  at R01 in every hall, so over all locations the name alone is not enough
   *  to tell two of them apart. */
  readonly dcName = (dcId: string): string =>
    this.datacenters().find((dc) => dc.id === dcId)?.name ?? '';

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

  readonly currentDcName = computed(() => {
    if (!this.selectedDcId()) return 'all locations';
    return this.datacenters().find((dc) => dc.id === this.selectedDcId())?.name ?? '';
  });

  constructor() {
    // Every location, until you pick one. Opening on the first data center was
    // not a choice anybody made, only the order the list happened to come back
    // in, and free space is a question about the whole estate rather than about
    // one hall.
    effect(() => {
      const dcs = this.datacenters();
      untracked(() => {
        if (this.selectedDcId()) this.rackList.load(this.selectedDcId());
        else this.rackList.loadAll(dcs.map((dc) => dc.id));
      });
    });
    // And it follows a later pick, which reads that one building.
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
