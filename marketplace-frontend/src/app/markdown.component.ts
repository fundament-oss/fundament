import { Component, computed, input, ChangeDetectionStrategy } from '@angular/core';
import { Marked, type Tokens } from 'marked';

// Publisher-written descriptions are markdown. The output goes through
// [innerHTML], so Angular's sanitizer strips scripts, event handlers and
// javascript: URLs as well.
//
// Raw HTML in the source is shown as the text it is rather than passed
// through. Descriptions published before they were read as markdown are plain
// text, and one saying "Create a <CustomResource> object" would otherwise hand
// the "tag" to the sanitizer and lose it without a trace. Indented code blocks
// are off for the same reason: an indented line of plain text is not code.
// Fenced blocks still are.
//
// The console renders the same description field with the same rules
// (console-frontend/src/app/markdown.ts); a change here belongs there too.
function escapeHtml(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('"', '&quot;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;');
}

function createParser(sectionLevel: number) {
  return new Marked({
    gfm: true,
    breaks: false,
    tokenizer: {
      code() {
        return undefined;
      },
    },
    walkTokens(token) {
      // The page owns the heading of the section the description sits in, so
      // none of the description's may be at that level or above it. Publishers
      // start at `##`; a `#` is taken to mean the same.
      if (token.type === 'heading') {
        const heading = token as Tokens.Heading;
        heading.depth = Math.min(Math.max(heading.depth, 2) + sectionLevel - 1, 6);
      }
    },
    renderer: {
      html({ text }) {
        return escapeHtml(text);
      },
      link({ href, title, tokens }) {
        const text = this.parser.parseInline(tokens);
        const titleAttr = title ? ` title="${escapeHtml(title)}"` : '';
        return `<a href="${escapeHtml(href)}"${titleAttr} target="_blank" rel="noopener noreferrer">${text}</a>`;
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

  // The level of the heading of the section this sits in: at 2, `##` in the
  // source becomes <h3>.
  sectionLevel = input(2);

  private parser = computed(() => createParser(this.sectionLevel()));

  html = computed(() => this.parser().parse(this.source(), { async: false }));
}
