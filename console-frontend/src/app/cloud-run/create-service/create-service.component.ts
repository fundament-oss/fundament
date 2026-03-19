import { ChangeDetectionStrategy, Component, computed, signal, inject } from '@angular/core';
import { FormBuilder, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { toSignal } from '@angular/core/rxjs-interop';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerCloud,
  tablerBrandGithub,
  tablerCode,
  tablerChevronDown,
  tablerChevronUp,
} from '@ng-icons/tabler-icons';

const NAMESPACE_MAP: Record<string, string[]> = {
  'cluster-1': ['default', 'production'],
  'cluster-2': ['default', 'staging'],
};

@Component({
  selector: 'app-cloud-run-create-service',
  imports: [ReactiveFormsModule, RouterLink, NgIcon],
  viewProviders: [
    provideIcons({
      tablerCloud,
      tablerBrandGithub,
      tablerCode,
      tablerChevronDown,
      tablerChevronUp,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './create-service.component.html',
})
export default class CreateServiceComponent {
  private fb = inject(FormBuilder);

  private router = inject(Router);

  // Panel and tab state
  panelOpen = signal(true);

  togglePanel() {
    this.panelOpen.update((v) => !v);
  }

  activeTab = signal<'containers' | 'networking' | 'security'>('containers');

  // Reactive form
  form: FormGroup = this.fb.group({
    deploymentSource: ['container-image'],
    containerImageUrl: [''],
    serviceName: ['', [Validators.required, Validators.pattern(/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/)]],
    cluster: ['', Validators.required],
    namespace: ['', Validators.required],
    authentication: ['public'],
    scaling: ['auto'],
    minInstances: [0],
    maxInstances: [null],
    manualInstances: [1],
    ingress: ['all'],
    ingressAllowLoadBalancers: [false],
    containerPort: [8080],
    containerName: [''],
    containerCommand: [''],
    containerArgs: [''],
    memory: ['512Mi'],
    cpu: ['1'],
    gpu: [false],
    requestTimeout: [300],
    maxConcurrentRequests: [80],
  });

  // Bridge cluster form control into signal graph for computed()
  private clusterValue = toSignal(this.form.get('cluster')!.valueChanges, {
    initialValue: this.form.get('cluster')!.value as string,
  });

  private serviceNameValue = toSignal(this.form.get('serviceName')!.valueChanges, {
    initialValue: '',
  });

  private namespaceValue = toSignal(this.form.get('namespace')!.valueChanges, {
    initialValue: '',
  });

  // Derived namespace options based on selected cluster
  availableNamespaces = computed<string[]>(() => NAMESPACE_MAP[this.clusterValue()] ?? []);

  // Computed endpoint URL
  endpointUrl = computed<string>(() => {
    const name = this.serviceNameValue();
    const ns = this.namespaceValue();
    if (!name || !ns) return '—';
    return `https://${name}.${ns}.svc.cluster.local`;
  });

  readonly clusters = ['cluster-1', 'cluster-2'];

  readonly memoryOptions = ['128Mi', '256Mi', '512Mi', '1Gi', '2Gi', '4Gi', '8Gi', '16Gi', '32Gi'];

  readonly cpuOptions = ['1', '2', '4', '6', '8'];

  get serviceName() {
    return this.form.get('serviceName');
  }

  onClusterChange() {
    this.form.get('namespace')!.reset('');
  }

  onSubmit(): void {
    this.router.navigate(['/cloud-run/services']);
  }

  onCancel() {
    this.router.navigate(['/cloud-run/services']);
  }
}
