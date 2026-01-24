import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { TitleService } from '../title.service';

@Component({
  selector: 'app-namespace-settings',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './namespace-settings.component.html',
})
export class NamespaceSettingsComponent {
  private titleService = inject(TitleService);

  constructor() {
    this.titleService.setTitle('Namespace Settings');
  }
}
