import { ChangeDetectionStrategy, Component, CUSTOM_ELEMENTS_SCHEMA, input, output } from '@angular/core';
import DialogSyncDirective from '../../dialog-sync.directive';

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
