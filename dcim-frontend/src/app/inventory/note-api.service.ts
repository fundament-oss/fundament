import { Injectable, inject } from '@angular/core';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { NoteEntityType } from '../../generated/v1/common_pb';
import type { Note as ProtoNote } from '../../generated/v1/note_pb';
import type { NoteComment } from './inventory';
import { NOTE_CLIENT } from '../../connect/tokens';

@Injectable({ providedIn: 'root' })
export default class NoteApiService {
  private readonly client = inject(NOTE_CLIENT);

  /** Maps an API note onto the UI comment model used by the notes card. */
  static mapNote(n: ProtoNote): NoteComment {
    const created = n.created ? timestampDate(n.created) : new Date();
    const daysAgo = Math.max(0, Math.floor((Date.now() - created.getTime()) / 86_400_000));
    return {
      id: n.id,
      author: n.createdBy,
      initials: NoteApiService.initials(n.createdBy),
      daysAgo,
      content: n.body,
    };
  }

  /** Derives up to two uppercase initials from an author name. */
  private static initials(name: string): string {
    const parts = name.trim().split(/\s+/).filter(Boolean);
    if (parts.length === 0) return '?';
    return parts
      .slice(0, 2)
      .map((p) => p[0]!.toUpperCase())
      .join('');
  }

  listNotesForAsset(assetId: string) {
    return this.client.listNotes({ entityType: NoteEntityType.ASSET, entityId: assetId });
  }

  listNotesForPlacement(placementId: string) {
    return this.client.listNotes({ entityType: NoteEntityType.PLACEMENT, entityId: placementId });
  }

  listNotesForTask(taskId: string) {
    return this.client.listNotes({ entityType: NoteEntityType.TASK, entityId: taskId });
  }

  // The author is not sent by the client: the backend attributes the note to
  // the authenticated caller (derived from the JWT) so it cannot be spoofed.
  createNoteForTask(taskId: string, body: string) {
    return this.client.createNote({
      entityType: NoteEntityType.TASK,
      entityId: taskId,
      body,
    });
  }

  createNoteForPlacement(placementId: string, body: string) {
    return this.client.createNote({
      entityType: NoteEntityType.PLACEMENT,
      entityId: placementId,
      body,
    });
  }
}
