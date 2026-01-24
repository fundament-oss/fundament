import { Component, inject, ViewChild, ElementRef, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { TitleService } from '../title.service';
import { CheckmarkIconComponent, CloseIconComponent, EditIconComponent } from '../icons';

interface Namespace {
  id: string;
  name: string;
  created: string;
}

@Component({
  selector: 'app-namespace-settings',
  standalone: true,
  imports: [
    CommonModule,
    FormsModule,
    CheckmarkIconComponent,
    CloseIconComponent,
    EditIconComponent,
  ],
  templateUrl: './namespace-settings.component.html',
})
export class NamespaceSettingsComponent {
  private titleService = inject(TitleService);

  @ViewChild('nameInput') nameInput?: ElementRef<HTMLInputElement>;

  // Mock namespace data
  namespace = signal<Namespace>({
    id: '660e8400-e29b-41d4-a716-446655440000',
    name: 'production',
    created: new Date().toISOString(),
  });

  isEditing = signal(false);
  editingName = signal('');
  loading = signal(false);
  error = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Namespace Settings');
  }

  startEdit() {
    const currentNamespace = this.namespace();
    if (currentNamespace) {
      this.isEditing.set(true);
      this.editingName.set(currentNamespace.name);

      // Focus the input field after Angular updates the view
      setTimeout(() => {
        this.nameInput?.nativeElement.focus();
      });
    }
  }

  cancelEdit() {
    this.isEditing.set(false);
    this.editingName.set('');
  }

  saveEdit() {
    const currentNamespace = this.namespace();
    const nameToSave = this.editingName();

    if (!nameToSave.trim() || !currentNamespace) {
      return;
    }

    this.loading.set(true);
    this.error.set(null);

    // Simulate API call with timeout
    setTimeout(() => {
      // Update the local namespace with the new name
      this.namespace.set({
        ...currentNamespace,
        name: nameToSave.trim(),
      });
      this.isEditing.set(false);
      this.editingName.set('');
      this.loading.set(false);
    }, 500);
  }

  formatDate(dateString: string): string {
    try {
      return new Date(dateString).toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
      });
    } catch {
      return dateString;
    }
  }
}
