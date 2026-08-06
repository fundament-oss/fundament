import { Component, inject, ChangeDetectionStrategy, CUSTOM_ELEMENTS_SCHEMA } from '@angular/core';
import { Router } from '@angular/router';
import SheetSyncDirective from '../sheet-sync.directive';

/**
 * The permission reference, as a route of its own so the link that opens it can
 * be a real link: shareable, openable in a new tab, and closed by the browser's
 * back button.
 */
@Component({
  selector: 'app-permissions-sheet',
  imports: [SheetSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './permissions-sheet.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class PermissionsSheetComponent {
  private router = inject(Router);

  onClose(): void {
    this.router.navigateByUrl('/organization/members');
  }
}
