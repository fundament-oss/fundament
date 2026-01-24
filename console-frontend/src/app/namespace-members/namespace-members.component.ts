import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { TitleService } from '../title.service';

@Component({
  selector: 'app-namespace-members',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './namespace-members.component.html',
})
export class NamespaceMembersComponent {
  private titleService = inject(TitleService);

  constructor() {
    this.titleService.setTitle('Namespace Members');
  }
}
