import {
  Component,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
  output,
  viewChild,
  ElementRef,
} from '@angular/core';
import DialogSyncDirective from '../dialog-sync.directive';
import focusFirstModalInput from '../modal-focus';

interface Cluster {
  id: string;
  name: string;
  installed: boolean;
}

@Component({
  selector: 'app-install-plugin-modal',
  imports: [DialogSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './install-plugin-modal.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class InstallPluginModalComponent {
  pluginName = input('');

  clusters = input<Cluster[]>([]);

  show = input(false);

  closeModal = output<void>();

  install = output<string>();

  dialogRef = viewChild<ElementRef<HTMLElement>>('dialog');

  onOpen(): void {
    const el = this.dialogRef()?.nativeElement;
    if (el) focusFirstModalInput(el);
  }

  onClose(): void {
    this.closeModal.emit();
  }

  onInstall(clusterId: string): void {
    this.install.emit(clusterId);
  }
}
