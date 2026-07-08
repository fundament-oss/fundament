import {
  Component,
  inject,
  signal,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ReactiveFormsModule, FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import PluginDevelopmentService, {
  type SideloadCluster,
} from '../plugin-development/plugin-development.service';

@Component({
  selector: 'app-plugin-sideload',
  imports: [ReactiveFormsModule, RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './plugin-sideload.component.html',
})
export default class PluginSideloadComponent implements OnInit {
  private titleService = inject(TitleService);

  private toastService = inject(ToastService);

  private router = inject(Router);

  private fb = inject(FormBuilder);

  private service = inject(PluginDevelopmentService);

  clusters = signal<SideloadCluster[]>([]);

  formSubmitted = signal(false);

  submitting = signal(false);

  sideloadForm: FormGroup;

  constructor() {
    this.titleService.setTitle('Sideload plugin');

    this.sideloadForm = this.fb.group({
      image: ['', Validators.required],
      version: ['', Validators.required],
      displayName: [''],
      description: [''],
      clusterId: ['', Validators.required],
    });
  }

  async ngOnInit() {
    const clusters = await this.service.listClusters();
    this.clusters.set(clusters);
    // Default to the development cluster when one exists.
    const dev = clusters.find((c) => c.isDevelopment) ?? clusters[0];
    if (dev) {
      this.sideloadForm.get('clusterId')?.setValue(dev.id);
    }
  }

  get image() {
    return this.sideloadForm.get('image');
  }

  get version() {
    return this.sideloadForm.get('version');
  }

  onInput(controlName: string, event: Event) {
    const value = (event as CustomEvent<{ value: string }>).detail.value;
    this.sideloadForm.get(controlName)?.setValue(value);
    this.sideloadForm.get(controlName)?.markAsDirty();
  }

  async onSubmit(event?: Event) {
    event?.preventDefault();
    this.formSubmitted.set(true);

    if (this.sideloadForm.invalid) {
      this.sideloadForm.markAllAsTouched();
      return;
    }

    this.submitting.set(true);
    const value = this.sideloadForm.value;
    await this.service.sideload({
      image: value.image,
      version: value.version,
      displayName: value.displayName || undefined,
      description: value.description || undefined,
      clusterId: value.clusterId,
    });

    const cluster = this.clusters().find((c) => c.id === value.clusterId);
    this.toastService.success(
      `Sideloading ${value.image} onto ${cluster?.name ?? 'the selected cluster'}`,
    );
    this.router.navigate(['/plugins/manage']);
  }

  onCancel() {
    this.router.navigate(['/plugins/manage']);
  }
}
