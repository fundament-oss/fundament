import { Component, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, ActivatedRoute } from '@angular/router';
import { TitleService } from '../title.service';
import { SharedPluginsFormComponent } from '../shared-plugins-form/shared-plugins-form.component';
import { ErrorIconComponent } from '../icons';

@Component({
  selector: 'app-cluster-plugins',
  standalone: true,
  imports: [CommonModule, SharedPluginsFormComponent, ErrorIconComponent],
  templateUrl: './cluster-plugins.component.html',
})
export class ClusterPluginsComponent {
  private titleService = inject(TitleService);
  private router = inject(Router);
  private route = inject(ActivatedRoute);

  private clusterId = '';
  errorMessage = signal<string | null>(null);
  isSubmitting = signal(false);

  constructor() {
    this.titleService.setTitle('Cluster plugins');
    this.clusterId = this.route.snapshot.paramMap.get('id') || '';
  }

  async onFormSubmit(data: { preset: string; plugins: string[] }) {
    if (this.isSubmitting()) return;

    this.errorMessage.set(null);
    this.isSubmitting.set(true);

    try {
      // Note: The UpdateCluster API doesn't currently support updating plugins
      // This is typically a create-time only configuration
      // For now, we'll just log the attempt and show an informational error
      console.log('Attempting to update cluster plugins:', data);

      // Simulate an API call that would fail since plugins aren't updateable
      this.errorMessage.set(
        'Plugin configuration cannot be updated after cluster creation. This is a limitation of the current API.',
      );

      // Uncomment this when the API supports plugin updates:
      // await this.organizationApi.updateCluster({
      //   clusterId: this.clusterId,
      //   pluginIds: data.plugins,
      // });
      // this.router.navigate(['/clusters', this.clusterId]);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to update cluster plugins';
      this.errorMessage.set(message);
    } finally {
      this.isSubmitting.set(false);
    }
  }

  onCancel() {
    this.router.navigate(['/clusters', this.clusterId]);
  }
}
