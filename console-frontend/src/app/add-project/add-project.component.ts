import { Component, ViewChild, ElementRef, AfterViewInit, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ReactiveFormsModule, FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { ErrorIconComponent } from '../icons';
import { PROJECT } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import { CreateProjectRequestSchema } from '../../generated/v1/project_pb';
import { firstValueFrom } from 'rxjs';

@Component({
  selector: 'app-add-project',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, ErrorIconComponent],
  templateUrl: './add-project.component.html',
})
export class AddProjectComponent implements AfterViewInit {
  @ViewChild('projectNameInput') projectNameInput!: ElementRef<HTMLInputElement>;

  private titleService = inject(TitleService);
  private router = inject(Router);
  private fb = inject(FormBuilder);
  private client = inject(PROJECT);
  private toastService = inject(ToastService);

  projectForm: FormGroup;
  errorMessage = signal<string | null>(null);
  isCreating = signal<boolean>(false);

  constructor() {
    this.titleService.setTitle('Add project');

    this.projectForm = this.fb.group({
      name: [
        '',
        [
          Validators.required,
          Validators.maxLength(63),
          Validators.pattern(/^[a-z]([-a-z0-9]*[a-z0-9])?$/),
        ],
      ],
    });
  }

  ngAfterViewInit() {
    this.projectNameInput.nativeElement.focus();
  }

  get name() {
    return this.projectForm.get('name');
  }

  getNameError(): string {
    if (this.name?.hasError('required')) {
      return 'The project name is required.';
    }
    if (this.name?.hasError('maxlength')) {
      return 'The project name must not exceed 63 characters.';
    }
    if (this.name?.hasError('pattern')) {
      return `The project name must contain only lowercase alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character.`;
    }
    return '';
  }

  async onSubmit() {
    if (this.projectForm.invalid) {
      this.projectForm.markAllAsTouched();
      this.scrollToFirstError();
      return;
    }

    this.errorMessage.set(null);
    this.isCreating.set(true);

    try {
      const request = create(CreateProjectRequestSchema, {
        name: this.projectForm.value.name,
      });

      await firstValueFrom(this.client.createProject(request));

      this.toastService.info('Project created successfully.');
      this.router.navigate(['/projects']);
    } catch (error) {
      console.error('Failed to create project:', error);
      this.errorMessage.set(
        error instanceof Error ? error.message : 'Failed to create project. Please try again.',
      );
    } finally {
      this.isCreating.set(false);
    }
  }

  private scrollToFirstError() {
    setTimeout(() => {
      const firstInvalidControl = document.querySelector('.ng-invalid:not(form)');
      if (firstInvalidControl) {
        firstInvalidControl.scrollIntoView({ behavior: 'smooth' });
        (firstInvalidControl as HTMLElement).focus();
      }
    }, 0);
  }

  onCancel() {
    this.router.navigate(['/projects']);
  }
}
