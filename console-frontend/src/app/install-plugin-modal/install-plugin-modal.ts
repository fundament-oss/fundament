import { Component, EventEmitter, Input, Output, ChangeDetectionStrategy } from '@angular/core';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerCheck } from '@ng-icons/tabler-icons';
import ModalComponent from '../modal/modal.component';

interface Cluster {
  id: string;
  name: string;
  installed: boolean;
}

@Component({
  selector: 'app-install-plugin-modal',
  imports: [NgIcon, ModalComponent],
  viewProviders: [
    provideIcons({
      tablerCheck,
    }),
  ],
  templateUrl: './install-plugin-modal.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class InstallPluginModalComponent {
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
