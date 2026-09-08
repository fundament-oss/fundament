import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
  output,
} from '@angular/core';
import DialogSyncDirective from '../../dialog-sync.directive';

import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/container';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/modal-dialog';
import '@nldd/design-system/spacer';
// Reusable "delete this resource?" confirmation modal for the native plugin
// resource views (list + detail). The parent owns the delete call and its
// loading/error state; this component only renders the confirmation and emits
// confirm/close.
@Component({
  selector: 'app-resource-delete-modal',
  imports: [DialogSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './resource-delete-modal.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ResourceDeleteModalComponent {
  show = input(false);

  resourceKind = input('');

  resourceName = input('');

  deleting = input(false);

  errorMessage = input<string | null>(null);

  confirm = output<void>();

  dismiss = output<void>();
}
