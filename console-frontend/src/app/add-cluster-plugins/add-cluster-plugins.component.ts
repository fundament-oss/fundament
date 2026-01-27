import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { SharedPluginsFormComponent } from '../shared-plugins-form/shared-plugins-form.component';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerArrowRight } from '@ng-icons/tabler-icons';
import { ClusterWizardStateService } from '../add-cluster-wizard-layout/cluster-wizard-state.service';

@Component({
  selector: 'app-add-cluster-plugins',
  standalone: true,
  imports: [CommonModule, SharedPluginsFormComponent, RouterLink, NgIcon],
  viewProviders: [
    provideIcons({
      tablerArrowRight,
    }),
  ],
  templateUrl: './add-cluster-plugins.component.html',
})
export class AddClusterPluginsComponent {
  private titleService = inject(TitleService);
  private router = inject(Router);
  private stateService = inject(ClusterWizardStateService);

  constructor() {
    this.titleService.setTitle('Add cluster plugins');
  }

  onFormSubmit(data: { preset: string; plugins: string[] }) {
    console.log('Creating cluster with data:', data);

    // Save plugins to state
    this.stateService.updatePlugins({
      preset: data.preset,
      plugins: data.plugins,
    });
    this.stateService.markStepCompleted(2);

    // Navigate to the next step
    this.router.navigate(['/add-cluster/summary']);
  }
}
