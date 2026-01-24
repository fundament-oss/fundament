import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { TitleService } from '../title.service';

@Component({
  selector: 'app-project-settings',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './project-settings.component.html',
})
export class ProjectSettingsComponent {
  private titleService = inject(TitleService);

  constructor() {
    this.titleService.setTitle('Project Settings');
  }
}
