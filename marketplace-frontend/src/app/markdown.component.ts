import { Component, computed, input, ChangeDetectionStrategy } from '@angular/core';
import { Marked, type Tokens } from 'marked';

// Publisher-written descriptions are markdown. The output goes through
// [innerHTML], so Angular's sanitizer strips scripts, event handlers and
// javascript: URLs from whatever raw HTML the source carries.
function escapeAttribute(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('"', '&quot;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;');
}

function createParser(headingOffset: number) {
  return new Marked({
    gfm: true,
    breaks: false,
    walkTokens(token) {
      // The page already owns the surrounding headings, so a `##` in the
      // description must not sit at the same level as the section it lives in.
      if (token.type === 'heading') {
        const heading = token as Tokens.Heading;
        heading.depth = Math.min(heading.depth + headingOffset, 6);
      }
    },
    renderer: {
      link({ href, title, tokens }) {
        const text = this.parser.parseInline(tokens);
        const titleAttr = title ? ` title="${escapeAttribute(title)}"` : '';
        return `<a href="${escapeAttribute(href)}"${titleAttr} target="_blank" rel="noopener noreferrer">${text}</a>`;
      },
    },
  });
}

@Component({
  selector: 'app-markdown',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'markdown block' },
  template: `<div [innerHTML]="html()"></div>`,
})
export default class MarkdownComponent {
  source = input.required<string>();

  // How many levels to push markdown headings down: 1 turns `#` into <h2>.
  headingOffset = input(1);

  private parser = computed(() => createParser(this.headingOffset()));

  html = computed(() => this.parser().parse(this.source(), { async: false }));
}
