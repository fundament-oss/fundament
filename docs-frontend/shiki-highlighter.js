// Extend the base class astro-asciidoc re-exports: it pins its own
// @asciidoctor/core, and the highlighter is registered with that copy.
import { SyntaxHighlighterBase } from 'astro-asciidoc';
import * as shiki from 'shiki';

/**
 * Asciidoctor syntax highlighter that renders source blocks with Shiki at
 * build time. Register it under the name used in the `source-highlighter`
 * document attribute.
 */
export class ShikiHighlighter extends SyntaxHighlighterBase {
  /**
   * Loads Shiki up front: the Asciidoctor version astro-asciidoc pins calls
   * `highlight()` synchronously.
   *
   * @param {string} name
   * @param {Parameters<typeof shiki.getSingletonHighlighter>[0]} options
   */
  static async create(name, options) {
    return new ShikiHighlighter(name, await shiki.getSingletonHighlighter(options), options);
  }

  /**
   * @param {string} name
   * @param {shiki.Highlighter} highlighter
   * @param {Parameters<typeof shiki.getSingletonHighlighter>[0]} options
   */
  constructor(name, highlighter, options) {
    super(name);
    this.highlighter = highlighter;
    this.theme = options?.themes?.[0] ?? 'min-light';
  }

  handlesHighlighting() {
    return true;
  }

  highlight(_node, source, lang) {
    return this.highlighter.codeToHtml(source, { lang, theme: this.theme });
  }

  // Shiki already returns a complete <pre><code> block, so skip the base
  // class's wrapper around the highlighted content.
  format(node) {
    return node.content();
  }
}
