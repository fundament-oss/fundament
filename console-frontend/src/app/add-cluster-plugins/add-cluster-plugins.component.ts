import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Title } from '@angular/platform-browser';
import { Router, RouterLink } from '@angular/router';
import { SharedPluginsFormComponent } from '../shared-plugins-form/shared-plugins-form.component';
import { ArrowRightIconComponent } from '../icons';

@Component({
  selector: 'app-add-cluster-plugins',
  standalone: true,
  imports: [CommonModule, SharedPluginsFormComponent, RouterLink, ArrowRightIconComponent],
  templateUrl: './add-cluster-plugins.component.html',
})
export class AddClusterPluginsComponent {
  private titleService = inject(Title);
  private router = inject(Router);

  constructor() {
    this.titleService.setTitle('Add cluster plugins — Fundament Console');
  }

  onFormSubmit(data: { preset: string; plugins: string[] }) {
    console.log('Creating cluster with data:', data);

    this.router.navigate(['/add-cluster/summary']);
  }
}
