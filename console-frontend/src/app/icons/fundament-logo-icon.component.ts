import { Component, Input, ChangeDetectionStrategy } from '@angular/core';

@Component({
  selector: 'app-fundament-logo-icon',
  host: {
    class: 'contents',
  },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <svg xmlns="http://www.w3.org/2000/svg" [attr.class]="class" viewBox="0 0 24 24">
      <path
        fill="currentColor"
        d="M6 5h12a1 1 0 0 1 1 1a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1a1 1 0 0 1 1-1m15-3v2H3V2zm-6 6h2v14h-2zM7 8h2v14H7zm4 0h2v14h-2z"
      />
    </svg>
  `,
})
export class FundamentLogoIconComponent {
  @Input() class = '';
}
