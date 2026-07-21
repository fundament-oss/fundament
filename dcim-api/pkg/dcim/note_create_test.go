package dcim_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1/dcimv1connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTaskForNote(t *testing.T, env *testEnv, title string) string {
	t.Helper()

	return createTask(t, env, (&dcimv1.CreateTaskRequest_builder{
		Title:    title,
		Status:   dcimv1.TaskStatus_TASK_STATUS_READY,
		Priority: dcimv1.TaskPriority_TASK_PRIORITY_LOW,
		Category: dcimv1.TaskCategory_TASK_CATEGORY_HARDWARE,
	}).Build())
}

func listTaskNotes(t *testing.T, env *testEnv, taskID string) []*dcimv1.Note {
	t.Helper()

	client := dcimv1connect.NewNoteServiceClient(env.client(), env.server.URL)

	resp, err := client.ListNotes(context.Background(), connect.NewRequest(
		(&dcimv1.ListNotesRequest_builder{
			EntityType: dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_TASK,
			EntityId:   taskID,
		}).Build(),
	))
	require.NoError(t, err)

	return resp.Msg.GetNotes()
}

// The author is resolved from the JWT, never from the request, so a note is
// attributed to the caller's directory entry and cannot be pinned on anyone
// else.
func TestNoteService_CreateNote_AttributesToCaller(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewNoteServiceClient(env.client(), env.server.URL)

	// A second roster member the note must NOT be attributed to.
	createUser(t, env, "Someone Else", "else@example.com", "00000000-0000-0000-0000-0000000000ff")
	authorID := createUser(t, env, "Note Author", "author@example.com", env.subject)

	taskID := createTaskForNote(t, env, "Task With Notes")

	_, err := client.CreateNote(context.Background(), connect.NewRequest(
		(&dcimv1.CreateNoteRequest_builder{
			EntityType: dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_TASK,
			EntityId:   taskID,
			Body:       "disk swapped",
		}).Build(),
	))
	require.NoError(t, err)

	notes := listTaskNotes(t, env, taskID)
	require.Len(t, notes, 1)

	assert.Equal(t, "disk swapped", notes[0].GetBody())
	assert.Equal(t, "Note Author", notes[0].GetCreatedBy())
	// The id is what a client joins onto the roster with; the display name alone
	// would collide between users who share a name.
	assert.Equal(t, authorID, notes[0].GetCreatedById())
}

// The roster is provisioned out of band, so a caller who is authenticated but
// absent from it must still be able to take notes — they just come out
// unattributed. Refusing here would break note-taking entirely in any
// environment where the directory has not been populated.
func TestNoteService_CreateNote_NoDirectoryEntry(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewNoteServiceClient(env.client(), env.server.URL)

	taskID := createTaskForNote(t, env, "Task Noted By A Stranger")

	_, err := client.CreateNote(context.Background(), connect.NewRequest(
		(&dcimv1.CreateNoteRequest_builder{
			EntityType: dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_TASK,
			EntityId:   taskID,
			Body:       "no roster entry here",
		}).Build(),
	))
	require.NoError(t, err)

	notes := listTaskNotes(t, env, taskID)
	require.Len(t, notes, 1)

	assert.Equal(t, "no roster entry here", notes[0].GetBody())
	assert.Empty(t, notes[0].GetCreatedBy(), "a caller outside the roster leaves the note unattributed")
}

// A soft-deleted directory entry reads as absent, so the note is written
// unattributed rather than failing on the (still valid) foreign key.
func TestNoteService_CreateNote_SoftDeletedAuthor(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewNoteServiceClient(env.client(), env.server.URL)

	userID := createUser(t, env, "Deleted Author", "gone@example.com", env.subject)
	_, err := env.adminPool.Exec(context.Background(),
		`UPDATE dcim.users SET deleted = now() WHERE id = $1`, userID)
	require.NoError(t, err)

	taskID := createTaskForNote(t, env, "Task Noted After Deletion")

	_, err = client.CreateNote(context.Background(), connect.NewRequest(
		(&dcimv1.CreateNoteRequest_builder{
			EntityType: dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_TASK,
			EntityId:   taskID,
			Body:       "written after the author was removed",
		}).Build(),
	))
	require.NoError(t, err)

	notes := listTaskNotes(t, env, taskID)
	require.Len(t, notes, 1)
	assert.Empty(t, notes[0].GetCreatedBy())
}
