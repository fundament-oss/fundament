import {
  Component,
  ChangeDetectionStrategy,
  inject,
  signal,
  ViewChild,
  ViewContainerRef,
  AfterViewInit,
  OnDestroy,
} from '@angular/core';
import { Subscription } from 'rxjs';
import ModalComponent from '../../modal/modal.component';
import PluginModalService from '../plugin-modal.service';
import PluginComponentRegistryService from '../plugin-component-registry.service';

/**
 * Portal that listens for PluginModalService.open() requests, loads the requested component
 * from the registry, and renders it inside the standard ModalComponent.
 *
 * Place this component once in the app shell template, above all route content.
 */
@Component({
  selector: 'app-plugin-modal-portal',
  imports: [ModalComponent],
  template: `
    <app-modal [show]="show()" [title]="title()" (modalClose)="onClose()">
      <ng-template #outlet />
    </app-modal>
  `,
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class PluginModalPortalComponent implements AfterViewInit, OnDestroy {
  @ViewChild('outlet', { read: ViewContainerRef }) private outlet!: ViewContainerRef;

  private modalService = inject(PluginModalService);

  private componentRegistry = inject(PluginComponentRegistryService);

  show = signal(false);

  title = signal('');

  private subscription?: Subscription;

  private closeModalListener = () => this.onClose();

  ngAfterViewInit(): void {
    window.addEventListener('plugin:close-modal', this.closeModalListener);

    this.subscription = this.modalService.open$.subscribe(async ({ componentName, context }) => {
      this.outlet.clear();
      this.show.set(true);
      this.title.set('');

      const type = await this.componentRegistry.load(componentName);
      if (!type) {
        // eslint-disable-next-line no-console
        console.error(`[PluginModalPortal] Component not registered: ${componentName}`);
        this.show.set(false);
        return;
      }

      const ref = this.outlet.createComponent(type);
      // Pass context to the component if it accepts a context input
      try {
        ref.setInput('context', context);
      } catch {
        // Component doesn't have a context input — that's fine
      }
    });
  }

  ngOnDestroy(): void {
    this.subscription?.unsubscribe();
    window.removeEventListener('plugin:close-modal', this.closeModalListener);
  }

  onClose(): void {
    this.show.set(false);
    this.outlet.clear();
    this.modalService.notifyClose();
  }
}
