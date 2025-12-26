import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { Title } from '@angular/platform-browser';
import { Router, RouterLink } from '@angular/router';
import { ProgressStepperComponent } from '../progress-stepper/progress-stepper.component';
import { ADD_CLUSTER_STEPS } from '../add-cluster/add-cluster.constants';

interface Plugin {
  id: string;
  name: string;
  description: string;
  selected: boolean;
}

@Component({
  selector: 'app-add-cluster-plugins',
  standalone: true,
  imports: [CommonModule, FormsModule, ProgressStepperComponent, RouterLink],
  templateUrl: './add-cluster-plugins.component.html',
  styleUrl: './add-cluster-plugins.component.css',
})
export class AddClusterPluginsComponent {
  private titleService = inject(Title);
  private router = inject(Router);

  // Progress stepper
  steps = ADD_CLUSTER_STEPS;
  currentStepIndex = 2;

  selectedPreset = 'havenplus'; // Default to Haven+ preset

  plugins: Plugin[] = [
    {
      id: 'alloy',
      name: 'Alloy',
      description:
        'Collects, processes, and sends logs, metrics, and traces (telemetry) to observability tools.',
      selected: true,
    },
    {
      id: 'cert-manager',
      name: 'cert-manager',
      description: 'Automates the requesting and renewal of TLS certificates.',
      selected: true,
    },
    {
      id: 'cloudnative-pg',
      name: 'Cloudnative-pg',
      description: 'Manages PostgreSQL clusters on Kubernetes.',
      selected: true,
    },
    {
      id: 'eck-operator',
      name: 'ECK operator',
      description: 'Manages Elasticsearch clusters and associated components within Kubernetes.',
      selected: true,
    },
    {
      id: 'grafana',
      name: 'Grafana',
      description: 'Visualizes metrics, logs, and traces in clear dashboards.',
      selected: true,
    },
    {
      id: 'istio-gateway',
      name: 'Istio gateway',
      description: 'Manages incoming traffic to services via configurable ingress policies.',
      selected: true,
    },
    {
      id: 'istio',
      name: 'Istio',
      description:
        'Controls service-to-service communication, security, and observability within a service mesh.',
      selected: true,
    },
    {
      id: 'keycloak',
      name: 'Keycloak',
      description:
        'Provides identity and access management with support for SSO, OpenID Connect, and more.',
      selected: true,
    },
    {
      id: 'loki',
      name: 'Loki',
      description: 'Stores log files and makes them searchable.',
      selected: true,
    },
    {
      id: 'mimir',
      name: 'Mimir',
      description: 'Stores time series (metrics) in a scalable way.',
      selected: true,
    },
    {
      id: 'pinniped',
      name: 'Pinniped',
      description:
        'Provides secure authentication in Kubernetes environments via existing identity providers.',
      selected: true,
    },
    {
      id: 'sealed-secrets',
      name: 'Sealed secrets',
      description: 'Enables encrypted secrets to be safely stored in Git.',
      selected: true,
    },
    {
      id: 'tempo',
      name: 'Tempo',
      description:
        'Processes and visualizes tracing data to make dependencies and performance insights clear.',
      selected: true,
    },
  ];

  constructor() {
    this.titleService.setTitle('Add cluster plugins — Fundament Console');
  }

  onPresetChange() {
    if (this.selectedPreset === 'havenplus') {
      // Haven+ preset: select all plugins
      this.plugins.forEach((plugin) => (plugin.selected = true));
    } else if (this.selectedPreset === 'preset2') {
      // Preset #2: only Grafana and Pinniped
      this.plugins.forEach((plugin) => {
        plugin.selected = plugin.id === 'grafana' || plugin.id === 'pinniped';
      });
    }
    // For custom preset, don't change selections automatically
  }

  onSubmit() {
    const selectedPlugins = this.plugins.filter((plugin) => plugin.selected);
    const clusterData = {
      preset: this.selectedPreset,
      plugins: selectedPlugins.map((plugin) => plugin.id),
    };

    console.log('Creating cluster with data:', clusterData);

    this.router.navigate(['/add-cluster-summary']);
  }
}
