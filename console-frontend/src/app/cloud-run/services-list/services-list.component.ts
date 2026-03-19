import { ChangeDetectionStrategy, Component } from '@angular/core';
import { RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerCloud, tablerPlus } from '@ng-icons/tabler-icons';

interface MockService {
  name: string;
  cluster: string;
  namespace: string;
  url: string;
  status: 'Ready' | 'Deploying' | 'Error';
  created: string;
}

function statusBadgeClass(status: MockService['status']): string {
  switch (status) {
    case 'Ready':
      return 'badge badge-sm badge-green';
    case 'Deploying':
      return 'badge badge-sm badge-blue';
    case 'Error':
      return 'badge badge-sm badge-rose';
    default:
      throw new Error(`Unhandled status: ${status satisfies never}`);
  }
}

@Component({
  selector: 'app-cloud-run-services-list',
  imports: [RouterLink, NgIcon],
  viewProviders: [provideIcons({ tablerCloud, tablerPlus })],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './services-list.component.html',
})
export default class ServicesListComponent {
  readonly services: MockService[] = [
    {
      name: 'api-gateway',
      cluster: 'cluster-1',
      namespace: 'production',
      url: 'https://api-gateway.production.svc.cluster.local',
      status: 'Ready',
      created: '2026-03-15',
    },
    {
      name: 'image-processor',
      cluster: 'cluster-1',
      namespace: 'default',
      url: 'https://image-processor.default.svc.cluster.local',
      status: 'Deploying',
      created: '2026-03-18',
    },
    {
      name: 'legacy-notifier',
      cluster: 'cluster-2',
      namespace: 'staging',
      url: 'https://legacy-notifier.staging.svc.cluster.local',
      status: 'Error',
      created: '2026-03-10',
    },
  ];

  readonly statusBadgeClass = statusBadgeClass;
}
