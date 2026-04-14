import { Component, inject, signal, OnInit, ChangeDetectionStrategy } from '@angular/core';
import { Router, ActivatedRoute } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
import { TitleService } from '../title.service';
import { SharedPluginsFormComponent } from '../shared-plugins-form/shared-plugins-form.component';
import { CLUSTER } from '../../connect/tokens';
import { fetchClusterName } from '../utils/cluster-status';

@Component({
  selector: 'app-cluster-plugins',
  imports: [SharedPluginsFormComponent, NgIcon],
  viewProviders: [
    provideIcons({
      tablerCircleXFill,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './cluster-plugins.component.html',
})
export default class ClusterPluginsComponent implements OnInit {
  private titleService = inject(TitleService);

  private router = inject(Router);

  private route = inject(ActivatedRoute);

  private client = inject(CLUSTER);

  private clusterId = '';

  errorMessage = signal<string | null>(null);

  isSubmitting = signal(false);

  currentPluginIds = signal<string[]>([]);

  clusterName = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Cluster plugins');
    this.clusterId = this.route.snapshot.paramMap.get('id') || '';
  }

  async ngOnInit() {
    await fetchClusterName(this.client, this.clusterId).then((name) => this.clusterName.set(name));
    // TODO: fetch installs via kube-api-proxy once that flow is implemented.
    this.currentPluginIds.set([]);
  }

  async onFormSubmit(_data: { preset: string; plugins: string[] }) {
    if (this.isSubmitting()) return;

    // TODO: sync install changes via kube-api-proxy once that flow is implemented.
    this.errorMessage.set('Updating cluster plugins is temporarily unavailable');
  }

  onCancel() {
    this.router.navigate(['/clusters', this.clusterId]);
  }
}
