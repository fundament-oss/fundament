/**
 * Puts text on the clipboard, or throws when it could not.
 *
 * The clipboard API needs a secure context. The console is served over HTTPS
 * everywhere it is deployed, but a plain-HTTP port-forward for debugging is
 * not, and losing copy there loses the only way to get a token or an ID out.
 * There the text goes through a hidden textarea and `execCommand('copy')`,
 * which reports failure by returning false rather than throwing.
 */
export default async function copyToClipboard(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }

  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  try {
    textarea.select();
    if (!document.execCommand('copy')) {
      throw new Error('The browser refused to copy');
    }
  } finally {
    document.body.removeChild(textarea);
  }
}
