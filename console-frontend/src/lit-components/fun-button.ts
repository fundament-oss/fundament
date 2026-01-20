import { LitElement, html } from 'lit';
import { customElement, property } from 'lit/decorators.js';

@customElement('fun-button')
export class FunButton extends LitElement {
  @property({ type: String }) variant: 'primary' | 'secondary' | 'light' | 'remove' = 'primary';
  @property({ type: String }) routerLink?: string;
  @property({ type: Boolean }) disabled = false;

  override createRenderRoot() {
    return this;
  }

  override connectedCallback() {
    super.connectedCallback();
    this.updateClassName();
    this.addEventListener('click', this.handleClick);
  }

  override disconnectedCallback() {
    super.disconnectedCallback();
    this.removeEventListener('click', this.handleClick);
  }

  override updated(changedProperties: Map<string, unknown>) {
    super.updated(changedProperties);
    if (changedProperties.has('variant')) {
      this.updateClassName();
    }
  }

  private updateClassName() {
    this.className = `btn-${this.variant}`;
  }

  private handleClick = (e: Event) => {
    if (this.routerLink && !this.disabled) {
      e.preventDefault();
      // Dispatch a custom event that Angular can listen to
      this.dispatchEvent(
        new CustomEvent('navigate', {
          detail: { path: this.routerLink },
          bubbles: true,
          composed: true,
        })
      );
    }
  };

  override render() {
    return html`<slot></slot>`;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fun-button': FunButton;
  }
}
