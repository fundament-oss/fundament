import { ChangeDetectionStrategy, Component, computed, signal, inject } from '@angular/core';
import { FormBuilder, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import { toSignal } from '@angular/core/rxjs-interop';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerCloud,
  tablerBrandGithub,
  tablerCode,
  tablerChevronDown,
  tablerChevronUp,
} from '@ng-icons/tabler-icons';

@Component({
  selector: 'app-cloud-run-create-service',
  imports: [ReactiveFormsModule, NgIcon],
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
    region: ['eu-west-1', Validators.required],
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

  private serviceNameValue = toSignal(this.form.get('serviceName')!.valueChanges, {
    initialValue: '',
  });

  private regionValue = toSignal(this.form.get('region')!.valueChanges, {
    initialValue: this.form.get('region')!.value as string,
  });

  // Computed endpoint URL
  endpointUrl = computed<string>(() => {
    const name = this.serviceNameValue();
    const region = this.regionValue();
    if (!name || !region) return '';
    return `https://${name}.${region}.run.example.com`;
  });

  readonly regions = [
    { value: 'eu-west-1', label: 'Europe (Amsterdam)' },
    { value: 'eu-central-1', label: 'Europe (Frankfurt)' },
    { value: 'us-east-1', label: 'US East (Virginia)' },
    { value: 'us-west-1', label: 'US West (Oregon)' },
    { value: 'ap-southeast-1', label: 'Asia Pacific (Singapore)' },
  ];

  readonly memoryOptions = ['128Mi', '256Mi', '512Mi', '1Gi', '2Gi', '4Gi', '8Gi', '16Gi', '32Gi'];

  readonly cpuOptions = ['1', '2', '4', '6', '8'];

  get serviceName() {
    return this.form.get('serviceName');
  }

  onSubmit(): void {
    this.router.navigate(['/cloud-run/services']);
  }

  onCancel() {
    this.router.navigate(['/cloud-run/services']);
  }
}
