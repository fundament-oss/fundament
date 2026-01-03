import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router } from '@angular/router';
import { TitleService } from '../title.service';
import { SharedPluginsFormComponent } from '../shared-plugins-form/shared-plugins-form.component';

@Component({
  selector: 'app-cluster-plugins',
  standalone: true,
  imports: [CommonModule, SharedPluginsFormComponent],
  templateUrl: './cluster-plugins.component.html',
})
export class ClusterPluginsComponent {
  private titleService = inject(TitleService);
  private router = inject(Router);

  constructor() {
    this.titleService.setTitle('Cluster plugins');
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
