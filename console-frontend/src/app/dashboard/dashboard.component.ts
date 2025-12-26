import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { Title } from '@angular/platform-browser';

@Component({
  selector: 'app-dashboard',
  standalone: true,
  imports: [CommonModule, RouterLink],
  templateUrl: './dashboard.component.html',
  styleUrl: './dashboard.component.css',
})
export class DashboardComponent {
  private titleService = inject(Title);

  // Mock data for existing clusters
  clusters = [
    {
      name: 'production-cluster',
      status: 'running',
      region: 'NL1',
      projectCount: 3,
      nodePoolCount: 2,
    },
    {
      name: 'staging-cluster',
      status: 'provisioning',
      region: 'NL2',
      projectCount: 1,
      nodePoolCount: 2,
    },
  ];

  constructor() {
    this.titleService.setTitle('Dashboard — Fundament Console');
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
