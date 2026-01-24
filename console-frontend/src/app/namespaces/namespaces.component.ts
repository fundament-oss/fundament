import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { TitleService } from '../title.service';

@Component({
  selector: 'app-namespaces',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './namespaces.component.html',
})
export class NamespacesComponent {
  private titleService = inject(TitleService);

  constructor() {
    this.titleService.setTitle('Namespaces');
  }
}
