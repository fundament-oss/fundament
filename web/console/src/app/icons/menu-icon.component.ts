import { Component, Input } from '@angular/core';

@Component({
  selector: 'app-menu-icon',
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
        d="M4 6h16M4 12h16M4 18h16"
      />
    </svg>
  `,
})
export class MenuIconComponent {
  @Input() class = '';
}
