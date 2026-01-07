import { Component, Input } from '@angular/core';

@Component({
  selector: 'app-arrow-right-icon',
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
        d="M5 12h14m-4 4l4-4m-4-4l4 4"
      />
    </svg>
  `,
})
export class ArrowRightIconComponent {
  @Input() class = '';
}
