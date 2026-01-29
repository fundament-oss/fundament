import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ActivatedRoute } from '@angular/router';
import { TitleService } from '../title.service';

@Component({
  selector: 'app-namespace-members',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './namespace-members.component.html',
})
export class NamespaceMembersComponent implements OnInit {
  private titleService = inject(TitleService);
  private route = inject(ActivatedRoute);

  projectId = signal<string>('');
  namespaceId = signal<string>('');

  constructor() {
    this.titleService.setTitle('Namespace Members');
  }

  ngOnInit() {
    const id = this.route.snapshot.params['id'];
    const nsId = this.route.snapshot.params['namespaceId'];
    if (id) {
      this.projectId.set(id);
    }
    if (nsId) {
      this.namespaceId.set(nsId);
    }
  }
}
