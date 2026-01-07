import { Component, Input } from '@angular/core';

@Component({
  selector: 'app-external-link-icon',
  standalone: true,
  host: {
    class: 'contents',
  },
  template: `
    <svg xmlns="http://www.w3.org/2000/svg" [attr.class]="class" viewBox="0 0 24 24">
      <path
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"
      />
      <polyline
        points="15 3 21 3 21 9"
        stroke="currentColor"
        fill="none"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      <line
        x1="10"
        y1="14"
        x2="21"
        y2="3"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
    </svg>
  `,
})
export class ExternalLinkIconComponent {
  @Input() class = '';
}
