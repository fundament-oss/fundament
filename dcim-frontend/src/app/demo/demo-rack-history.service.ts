import { Injectable } from '@angular/core';
import RackHistoryService, { RackEvent } from '../racks/rack-history.service';
import { racks } from './fixtures';

/**
 * A rack's history, for the demo only.
 *
 * Made up, like the rest of the fixtures, and kept here rather than in the page:
 * a component that carries its own sample data shows it to real customers the
 * day somebody forgets it is there. It hangs on the first few racks by name, so
 * whichever ids the fixtures generate, the same halls have a history.
 */
const BY_RACK_NAME: Record<string, RackEvent[]> = {
  'R01-1': [
    { user: 'Ops Team', daysAgo: 6, description: 'Powered on after a maintenance window' },
    { user: 'Monitoring', daysAgo: 8, description: 'NL-00061 went offline, PSU fault detected' },
    { user: 'Daan Hofman', daysAgo: 21, description: 'Two servers fitted and connected' },
    { user: 'Iris Wolters', daysAgo: 60, description: 'Rack put into service' },
  ],
  'R01-2': [
    { user: 'Sem Bakker', daysAgo: 3, description: 'Patch panel fitted in U42' },
    { user: 'Yara Nijhuis', daysAgo: 34, description: 'Rack put into service' },
  ],
  'R05-1': [
    { user: 'Daan Hofman', daysAgo: 12, description: 'Cold aisle containment fitted' },
    { user: 'Ops Team', daysAgo: 45, description: 'Rack put into service' },
  ],
};

@Injectable()
export default class DemoRackHistoryService extends RackHistoryService {
  override readonly historyFor = (rackId: string): RackEvent[] => {
    const name = racks.find((rack) => rack.id === rackId)?.name ?? '';
    return BY_RACK_NAME[name] ?? [];
  };
}
