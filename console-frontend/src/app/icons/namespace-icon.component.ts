import { Component, Input } from '@angular/core';

@Component({
  selector: 'app-namespace-icon',
  standalone: true,
  host: {
    class: 'contents',
  },
  template: `
    <svg xmlns="http://www.w3.org/2000/svg" [attr.class]="class" viewBox="0 0 24 24">
      <path
        fill="none"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="2"
        d="M7 4H3v16h4M17 4h4v16h-4m-9-4h.01M12 16h.01M16 16h.01"
      />
    </svg>
  `,
})
export class NamespaceIconComponent {
  @Input() class = '';
}
