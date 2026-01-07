import { Component, Input } from '@angular/core';

@Component({
  selector: 'app-table-download-icon',
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
        d="M12.5 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v7.5M3 10h18M10 3v18m9-5v6m3-3l-3 3l-3-3"
      />
    </svg>
  `,
})
export class TableDownloadIconComponent {
  @Input() class = '';
}
