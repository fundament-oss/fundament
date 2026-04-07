import {
  Component,
  EventEmitter,
  Input,
  Output,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import ModalComponent from '../modal/modal.component';

interface Cluster {
  id: string;
  name: string;
  installed: boolean;
}

@Component({
  selector: 'app-install-plugin-modal',
  imports: [ModalComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
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
