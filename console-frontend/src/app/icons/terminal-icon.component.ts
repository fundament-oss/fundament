import { Component, Input } from '@angular/core';

@Component({
  selector: 'app-terminal-icon',
  standalone: true,
  template: `
    <svg xmlns="http://www.w3.org/2000/svg" [attr.class]="class" viewBox="0 0 24 24">
      <path
        fill="none"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="2"
        d="m5 7l5 5l-5 5m7 2h7"
      />
    </svg>
  `,
})
export class TerminalIconComponent {
  @Input() class = '';
}
