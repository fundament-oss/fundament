import { Component, ViewChild, ElementRef, AfterViewInit, inject, signal, ChangeDetectionStrategy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, RouterLink } from '@angular/router';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { PROJECT } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import { CreateProjectRequestSchema } from '../../generated/v1/project_pb';
import { firstValueFrom } from 'rxjs';
import { ErrorIconComponent } from '../icons';

@Component({
  selector: 'app-add-project',
  imports: [CommonModule, RouterLink, ReactiveFormsModule, ErrorIconComponent],
  templateUrl: './add-project.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class AddProjectComponent implements AfterViewInit {
  @ViewChild('projectNameInput') projectNameInput!: ElementRef<HTMLInputElement>;
  private titleService = inject(TitleService);
  private router = inject(Router);
  private fb = inject(FormBuilder);
  private client = inject(PROJECT);
  private toastService = inject(ToastService);

  errorMessage = signal<string | null>(null);
  isSubmitting = signal<boolean>(false);

  projectForm = this.fb.group({
    name: [
      '',
      [
        Validators.required,
        Validators.minLength(1),
        Validators.maxLength(63),
        Validators.pattern(/^[a-z]([-a-z0-9]*[a-z0-9])?$/),
      ],
    ],
  });

  constructor() {
    this.titleService.setTitle('Add project');
  }

  ngAfterViewInit() {
    // Focus the project name input after the view is initialized
    this.projectNameInput.nativeElement.focus();
  }

  async onSubmit() {
    if (this.projectForm.invalid) {
      this.projectForm.markAllAsTouched();
      return;
    }

    try {
      this.isSubmitting.set(true);
      this.errorMessage.set(null);

      const request = create(CreateProjectRequestSchema, {
        name: this.projectForm.value.name!,
      });

      const response = await firstValueFrom(this.client.createProject(request));

      this.toastService.success(`Project '${this.projectForm.value.name}' created successfully`);
      this.router.navigate(['/projects', response.projectId]);
    } catch (error) {
      console.error('Failed to create project:', error);
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to create project: ${error.message}`
          : 'Failed to create project',
      );
    } finally {
      this.isSubmitting.set(false);
    }
  }

  getNameError(): string {
    const nameControl = this.projectForm.get('name');
    if (nameControl?.hasError('required')) {
      return 'Project name is required.';
    }
    if (nameControl?.hasError('maxlength')) {
      return 'Project name must not exceed 63 characters.';
    }
    if (nameControl?.hasError('pattern')) {
      return 'Project name must contain only lowercase letters, numbers, and hyphens, start with a letter, and end with a letter or number.';
    }
    return '';
  }
}
