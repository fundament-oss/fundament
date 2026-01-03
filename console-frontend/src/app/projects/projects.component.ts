import {
  Component,
  inject,
  ViewChildren,
  QueryList,
  ElementRef,
  AfterViewInit,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import {
  ReactiveFormsModule,
  FormBuilder,
  FormArray,
  FormGroup,
  Validators,
  AbstractControl,
  ValidationErrors,
} from '@angular/forms';
import { Title } from '@angular/platform-browser';
import { Router } from '@angular/router';
import { PlusIconComponent, TrashIconComponent } from '../icons';

@Component({
  selector: 'app-projects',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, PlusIconComponent, TrashIconComponent],
  templateUrl: './projects.component.html',
})
export class ProjectsComponent implements AfterViewInit {
  @ViewChildren('projectNameInput') projectNameInputs!: QueryList<ElementRef<HTMLInputElement>>;
  private titleService = inject(Title);
  private router = inject(Router);
  private fb = inject(FormBuilder);

  // Clusters
  clusterNames = ['cluster-1', 'cluster-2', 'cluster-3'];

  // Form
  projectsForm: FormGroup;

  constructor() {
    this.titleService.setTitle('Projects — Fundament Console');

    this.projectsForm = this.fb.group({
      projects: this.fb.array([this.createProjectFormGroup(0)]),
    });
  }

  get projects(): FormArray {
    return this.projectsForm.get('projects') as FormArray;
  }

  createProjectFormGroup(projectIndex?: number): FormGroup {
    return this.fb.group({
      name: [
        this.generateProjectName(),
        [
          Validators.required,
          Validators.maxLength(63),
          Validators.pattern(/^[a-z]([-a-z0-9]*[a-z0-9])?$/),
          this.uniqueProjectNameValidator.bind(this),
        ],
      ],
      namespaces: this.fb.array([
        this.createNamespaceFormGroup(0, projectIndex ?? this.projects.length),
      ]),
    });
  }

  createNamespaceFormGroup(namespaceIndex = 0, projectIndex: number): FormGroup {
    // Default namespace name is 'default' only for the first namespace
    const defaultName = namespaceIndex === 0 ? 'default' : '';

    return this.fb.group({
      cluster: ['cluster-1', [Validators.required]],
      name: [
        defaultName,
        [
          Validators.required,
          Validators.pattern(/^[a-z]([-a-z0-9]*[a-z0-9])?$/),
          (control: AbstractControl) => this.uniqueNamespaceValidator(control, projectIndex),
          (control: AbstractControl) => this.namespaceLengthValidator(control, projectIndex),
        ],
      ],
    });
  }

  private generateProjectName(): string {
    const randomSuffix = Array.from({ length: 3 }, () =>
      String.fromCharCode(97 + Math.floor(Math.random() * 26)),
    ).join('');
    return `my-project-${randomSuffix}`;
  }

  private uniqueProjectNameValidator(control: AbstractControl): ValidationErrors | null {
    if (!control.value || !this.projectsForm) {
      return null;
    }

    const currentName = control.value.toLowerCase();
    const projects = this.projects?.controls || [];

    const hasDuplicate = projects.some(
      (project) =>
        project !== control.parent && project.get('name')?.value?.toLowerCase() === currentName,
    );

    return hasDuplicate ? { duplicate: true } : null;
  }

  private uniqueNamespaceValidator(
    control: AbstractControl,
    projectIndex: number,
  ): ValidationErrors | null {
    if (!control.value || !this.projectsForm) {
      return null;
    }

    // Check if the project exists in the array yet
    if (!this.projects.at(projectIndex)) {
      return null;
    }

    const currentName = control.value.toLowerCase();
    const currentCluster = control.parent?.get('cluster')?.value;
    const namespaces = this.getNamespaces(projectIndex)?.controls || [];

    const hasDuplicate = namespaces.some(
      (namespace) =>
        namespace !== control.parent &&
        namespace.get('name')?.value?.toLowerCase() === currentName &&
        namespace.get('cluster')?.value === currentCluster,
    );

    return hasDuplicate ? { duplicate: true } : null;
  }

  private namespaceLengthValidator(
    control: AbstractControl,
    projectIndex: number,
  ): ValidationErrors | null {
    if (!control.value || !this.projectsForm) {
      return null;
    }

    // Check if the project exists in the array yet
    if (!this.projects.at(projectIndex)) {
      return null;
    }

    const namespaceName = control.value;
    const projectName = this.projects.at(projectIndex)?.get('name')?.value || '';
    const combinedLength = namespaceName.length + projectName.length;

    return combinedLength > 61 ? { combinedLength: true } : null;
  }

  ngAfterViewInit() {
    this.projectNameInputs.first?.nativeElement.focus();
  }

  getNamespaces(projectIndex: number): FormArray {
    return this.projects.at(projectIndex).get('namespaces') as FormArray;
  }

  getProjectNameError(projectIndex: number): string {
    const nameControl = this.projects.at(projectIndex).get('name');
    if (nameControl?.hasError('required')) {
      return 'The project name is required.';
    }
    if (nameControl?.hasError('maxlength')) {
      return 'The project name must not exceed 63 characters.';
    }
    if (nameControl?.hasError('pattern')) {
      return `The project name must contain only lowercase alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character.`;
    }
    if (nameControl?.hasError('duplicate')) {
      return 'This project name is already in use. Please choose a unique name.';
    }
    return '';
  }

  getNamespaceNameError(projectIndex: number, namespaceIndex: number): string {
    const nameControl = this.getNamespaces(projectIndex).at(namespaceIndex).get('name');
    if (nameControl?.hasError('required')) {
      return 'The namespace name is required.';
    }
    if (nameControl?.hasError('pattern')) {
      return `The namespace name must contain only lowercase alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character.`;
    }
    if (nameControl?.hasError('duplicate')) {
      return 'This namespace name is already in use within this project and cluster. Please choose a unique name.';
    }
    if (nameControl?.hasError('combinedLength')) {
      return 'The combined length of the namespace name and project name must not exceed 61 characters.';
    }
    return '';
  }

  addProject() {
    this.projects.push(this.createProjectFormGroup(this.projects.length));
    this.revalidateProjectNames();
  }

  removeProject(index: number) {
    if (this.projects.length > 1) {
      this.projects.removeAt(index);
      this.revalidateProjectNames();
    }
  }

  addNamespace(projectIndex: number) {
    const namespaces = this.getNamespaces(projectIndex);
    const newIndex = namespaces.length;
    namespaces.push(this.createNamespaceFormGroup(newIndex, projectIndex));
    this.revalidateNamespaceNames(namespaces);
  }

  removeNamespace(projectIndex: number, namespaceIndex: number) {
    const namespaces = this.getNamespaces(projectIndex);
    if (namespaces.length > 1) {
      namespaces.removeAt(namespaceIndex);
      this.revalidateNamespaceNames(namespaces);
    }
  }

  private revalidateProjectNames() {
    this.projects.controls.forEach((control) => {
      control.get('name')?.updateValueAndValidity();
    });
  }

  private revalidateNamespaceNames(namespaces: FormArray) {
    namespaces.controls.forEach((control) => {
      control.get('name')?.updateValueAndValidity();
    });
  }

  onSubmit() {
    if (this.projectsForm.invalid) {
      this.projectsForm.markAllAsTouched();
      this.scrollToFirstError();
      return;
    }

    const projectData = this.projectsForm.value;
    console.log('Updating projects with data:', projectData);

    // For now, just reload the page (?)
    // In a real app, this would make an API call
    this.router.navigate(['/projects']);
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
}
