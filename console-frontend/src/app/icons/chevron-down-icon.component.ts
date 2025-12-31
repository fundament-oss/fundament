import { Component, Input } from '@angular/core';

@Component({
  selector: 'app-chevron-down-icon',
  standalone: true,
  template: `
    <svg [attr.class]="class" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="2"
        d="M19 9l-7 7-7-7"
      ></path>
    </svg>
  `,
})
export class ChevronDownIconComponent {
  @Input() class = '';
}
