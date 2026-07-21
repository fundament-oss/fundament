import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import NoteApiService from './note-api.service';
import { NoteSchema } from '../../generated/v1/note_pb';

describe('NoteApiService.mapNote', () => {
  it('maps an attributed note onto the comment model', () => {
    const comment = NoteApiService.mapNote(
      create(NoteSchema, {
        id: '019dcc00-0000-7000-8000-000000000005',
        body: 'Disk bay 3 LED is solid red.',
        createdBy: 'Jan de Vries',
        created: timestampFromDate(new Date(Date.now() - 2 * 86_400_000)),
      }),
    );

    expect(comment.author).toBe('Jan de Vries');
    expect(comment.initials).toBe('JD');
    expect(comment.daysAgo).toBe(2);
    expect(comment.content).toBe('Disk bay 3 LED is solid red.');
  });

  // The author is resolved server-side from created_by_id, so it is empty for a
  // note whose writer has no directory entry — and for every note predating the
  // FK, since migration 032 dropped the free-text column without a backfill. A
  // blank byline reads as a rendering fault rather than as missing authorship.
  it('names an unattributed note Unknown instead of rendering a blank byline', () => {
    const comment = NoteApiService.mapNote(
      create(NoteSchema, { id: 'n1', body: 'Spare disk is in storage room B.' }),
    );

    expect(comment.author).toBe('Unknown');
  });

  it('keeps the neutral ? avatar for an unattributed note', () => {
    // Derived from the raw name, not from the "Unknown" label — a "U" would
    // read as a real person's initial.
    const comment = NoteApiService.mapNote(create(NoteSchema, { id: 'n1', body: 'body' }));

    expect(comment.initials).toBe('?');
  });

  it('never reports a negative age for a note created just now', () => {
    const comment = NoteApiService.mapNote(
      create(NoteSchema, {
        id: 'n1',
        body: 'body',
        created: timestampFromDate(new Date(Date.now() + 5_000)),
      }),
    );

    expect(comment.daysAgo).toBe(0);
  });
});
