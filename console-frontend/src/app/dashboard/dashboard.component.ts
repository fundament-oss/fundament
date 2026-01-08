import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { PlusIconComponent, EyeIconComponent, ErrorIconComponent } from '../icons';
import { OrganizationApiService, ClusterSummary } from '../organization-api.service';

@Component({
  selector: 'app-dashboard',
  standalone: true,
  imports: [CommonModule, RouterLink, PlusIconComponent, EyeIconComponent, ErrorIconComponent],
  templateUrl: './dashboard.component.html',
})
export class DashboardComponent implements OnInit {
  private titleService = inject(TitleService);
  private organizationApi = inject(OrganizationApiService);

  clusters = signal<ClusterSummary[]>([]);
  errorMessage = signal<string>('');

  constructor() {
    this.titleService.setTitle('Dashboard');
  }

  async ngOnInit() {
    try {
      const clusters = await this.organizationApi.listClusters();
      this.clusters.set(clusters);
    } catch (error) {
      console.error('Failed to load clusters:', error);
      this.errorMessage.set(
        error instanceof Error ? error.message : 'Failed to load clusters. Please try again later.',
      );
    }
  }

  getStatusColor(status: string): string {
    const colors: Record<string, string> = {
      provisioning: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-950 dark:text-yellow-200',
      starting: 'bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-200',
      running: 'bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-200',
      upgrading: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-950 dark:text-indigo-200',
      error: 'bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-200',
      stopping: 'bg-orange-100 text-orange-800 dark:bg-orange-950 dark:text-orange-200',
      stopped: 'bg-gray-100 text-gray-800 dark:bg-gray-950 dark:text-gray-200',
    };
    return colors[status] || 'bg-gray-100 text-gray-800 dark:bg-gray-950 dark:text-gray-200';
  }
}
