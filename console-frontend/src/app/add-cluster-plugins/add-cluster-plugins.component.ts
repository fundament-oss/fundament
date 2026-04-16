import { Component, inject, ViewChild, ChangeDetectionStrategy, CUSTOM_ELEMENTS_SCHEMA } from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { SharedPluginsFormComponent } from '../shared-plugins-form/shared-plugins-form.component';
import { ClusterWizardStateService } from '../add-cluster-wizard-layout/cluster-wizard-state.service';

@Component({
  selector: 'app-add-cluster-plugins',
  imports: [SharedPluginsFormComponent, RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './add-cluster-plugins.component.html',
})
export default class AddClusterPluginsComponent {
  @ViewChild(SharedPluginsFormComponent) pluginsForm!: SharedPluginsFormComponent;

  private titleService = inject(TitleService);

  private router = inject(Router);

  private stateService = inject(ClusterWizardStateService);

  constructor() {
    this.titleService.setTitle('Add cluster plugins');
  }

  onFormSubmit(data: { preset: string; plugins: string[] }) {
    // Save plugins to state
    this.stateService.updatePlugins({
      preset: data.preset,
      plugins: data.plugins,
    });
    this.stateService.markStepCompleted(2);

    // Navigate to the next step
    this.router.navigate(['/clusters/add/summary']);
  }
}
