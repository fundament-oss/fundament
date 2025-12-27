import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Title } from '@angular/platform-browser';
import { Router, RouterLink } from '@angular/router';
import { SharedPluginsFormComponent } from '../shared-plugins-form/shared-plugins-form.component';

@Component({
  selector: 'app-cluster-plugins',
  standalone: true,
  imports: [CommonModule, SharedPluginsFormComponent, RouterLink],
  templateUrl: './cluster-plugins.component.html',
  styleUrl: './cluster-plugins.component.css',
})
export class ClusterPluginsComponent {
  private titleService = inject(Title);
  private router = inject(Router);

  constructor() {
    this.titleService.setTitle('Cluster plugins — Fundament Console');
  }

  onFormSubmit(data: { preset: string; plugins: string[] }) {
    console.log('Saving cluster plugin changes:', data);

    // For now, just navigate back to cluster overview
    // In a real app, this would make an API call
    this.router.navigate(['/cluster-overview']);
  }

  onCancel() {
    this.router.navigate(['/cluster-overview']);
  }
}
