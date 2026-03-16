import { Injectable } from '@angular/core';
import { Subject } from 'rxjs';

export interface PluginModalRequest {
  componentName: string;
  context: Record<string, unknown>;
}

/**
 * Service for opening plugin modal actions.
 *
 * Plugin components call `open(componentName, context)` to request a modal.
 * PluginModalPortalComponent subscribes to the `open$` observable, loads the
 * named component from PluginComponentRegistryService, and renders it inside
 * the app-modal wrapper.
 */
@Injectable({ providedIn: 'root' })
export default class PluginModalService {
  private openSubject = new Subject<PluginModalRequest>();

  /** Observable that emits whenever a plugin requests a modal. */
  readonly open$ = this.openSubject.asObservable();

  private closeSubject = new Subject<void>();

  /** Observable that emits when the portal closes the modal. */
  readonly close$ = this.closeSubject.asObservable();

  /**
   * Request opening a modal with the given registered component name.
   * The context object is passed to the component via `setInput('context', context)`.
   */
  open(componentName: string, context: Record<string, unknown> = {}): void {
    this.openSubject.next({ componentName, context });
  }

  /**
   * Called by the portal to notify that the modal was closed.
   */
  notifyClose(): void {
    this.closeSubject.next();
  }
}
