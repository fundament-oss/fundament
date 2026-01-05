import { Component, Input } from '@angular/core';

@Component({
  selector: 'app-plus-icon',
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
        d="M12 5v14m-7-7h14"
      />
    </svg>
  `,
})
export class PlusIconComponent {
  @Input() class = '';
}
