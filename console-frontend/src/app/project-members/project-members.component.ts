import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink, RouterLinkActive } from '@angular/router';
import { TitleService } from '../title.service';
import { PlusIconComponent, EditIconComponent, TrashIconComponent } from '../icons';

@Component({
  selector: 'app-project-members',
  standalone: true,
  imports: [
    CommonModule,
    RouterLink,
    RouterLinkActive,
    PlusIconComponent,
    EditIconComponent,
    TrashIconComponent,
  ],
  templateUrl: './project-members.component.html',
})
export class ProjectMembersComponent {
  private titleService = inject(TitleService);

  // Members data for the project
  members = {
    projects: [
      {
        name: 'Project #1',
        users: [
          { name: 'Alice Johnson', role: 'Project admin' },
          { name: 'Bob Johnson', role: 'Project member' },
          { name: 'Charlie Brown', role: 'Project member' },
        ],
      },
      {
        name: 'Project #2',
        users: [
          { name: 'David Wilson', role: 'Project admin' },
          { name: 'Emma Davis', role: 'Project member' },
        ],
      },
      {
        name: 'Project #3',
        users: [
          { name: 'Frank Miller', role: 'Project member' },
          { name: 'Grace Lee', role: 'Project member' },
        ],
      },
    ],
  };

  constructor() {
    this.titleService.setTitle('Project members');
  }
}
