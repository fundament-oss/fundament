import { Component, EventEmitter, Input, Output } from '@angular/core';
import { CommonModule } from '@angular/common';
import { CloseIconComponent, CheckmarkIconComponent } from '../icons';

interface Cluster {
  id: string;
  name: string;
  installed: boolean;
}

@Component({
  selector: 'app-install-plugin-modal',
  standalone: true,
  imports: [CommonModule, CloseIconComponent, CheckmarkIconComponent],
  templateUrl: './install-plugin-modal.html',
})
export class InstallPluginModalComponent {
  @Input() pluginName = '';
  @Input() clusters: Cluster[] = [];
  @Input() show = false;

  @Output() closeModal = new EventEmitter<void>();
  @Output() install = new EventEmitter<string>();

  onClose(): void {
    this.closeModal.emit();
  }

  onInstall(clusterId: string): void {
    this.install.emit(clusterId);
  }
}
