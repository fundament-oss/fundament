// Turning what is on screen into a file you can open in a spreadsheet.

/**
 * One cell, quoted so a comma, a quote or a newline inside it stays inside it.
 *
 * Everything is quoted rather than only the cells that need it: a spreadsheet
 * reads both the same way, and a rule with no exceptions is one less thing to
 * get wrong when a name suddenly contains a comma.
 */
function csvCell(value: string): string {
  return `"${value.replace(/"/g, '""')}"`;
}

/** Rows of already-formatted cells as one CSV document. */
export function toCsv(rows: string[][]): string {
  // CRLF, because Excel treats a bare LF as part of the last cell.
  return rows.map((row) => row.map(csvCell).join(',')).join('\r\n');
}

/**
 * Hands the CSV to the browser as a download.
 *
 * An anchor with a `download` and a blob URL, which is the only way to name a
 * file the page made itself. The element never has to be visible, but it does
 * have to be in the document for Firefox to honour the click.
 */
export function downloadCsv(filename: string, rows: string[][]): void {
  const blob = new Blob([toCsv(rows)], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

/** A name turned into something safe to put in a filename. */
export function slugify(value: string): string {
  return (
    value
      .toLowerCase()
      .replace(/\s+/g, '-')
      .replace(/[^a-z0-9-]/g, '')
      .replace(/-+/g, '-')
      .replace(/^-|-$/g, '') || 'export'
  );
}
