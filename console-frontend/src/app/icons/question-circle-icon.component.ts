import { Component, Input } from '@angular/core';

@Component({
  selector: 'app-question-circle-icon',
  standalone: true,
  host: {
    class: 'contents',
  },
  template: `
    <svg xmlns="http://www.w3.org/2000/svg" [attr.class]="class" viewBox="0 0 24 24">
      <g
        fill="none"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="2"
      >
        <path d="M12 16v.01M12 13a2 2 0 0 0 .914-3.782a1.98 1.98 0 0 0-2.414.483" />
        <path d="M3 12a9 9 0 1 0 18 0a9 9 0 1 0-18 0" />
      </g>
    </svg>
  `,
})
export class QuestionCircleIconComponent {
  @Input() class = '';
}
