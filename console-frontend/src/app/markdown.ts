import { Marked, type Tokens } from 'marked';

// Plugin descriptions are publisher-written markdown. The output goes through
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
// The marketplace renders the same description field with the same rules
// (marketplace-frontend/src/app/markdown.component.ts); a change here belongs
// there too.
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

const parsers = new Map<number, Marked>();

/**
 * Renders a plugin description to HTML, for a section whose own heading is at
 * `sectionLevel`: at 1, `##` in the source becomes <h2>.
 */
export default function renderMarkdown(source: string, sectionLevel: number): string {
  let parser = parsers.get(sectionLevel);
  if (!parser) {
    parser = createParser(sectionLevel);
    parsers.set(sectionLevel, parser);
  }
  return parser.parse(source, { async: false });
}
