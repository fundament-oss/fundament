package dcim_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1/dcimv1connect"
)

func createTaskForNote(t *testing.T, env *testEnv, title string) string {
	t.Helper()

	return createTask(t, env, (&dcimv1.CreateTaskRequest_builder{
		Title:    title,
		Status:   dcimv1.TaskStatus_TASK_STATUS_TODO,
		Priority: dcimv1.TaskPriority_TASK_PRIORITY_LOW,
		Tags:     []string{"hardware"},
	}).Build())
}

func listTaskNotes(t *testing.T, env *testEnv, taskID string) []*dcimv1.Note {
	t.Helper()

	client := dcimv1connect.NewNoteServiceClient(env.client(), env.server.URL)

	resp, err := client.ListNotes(context.Background(),
		(&dcimv1.ListNotesRequest_builder{
			EntityType: dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_TASK,
			EntityId:   taskID,
		}).Build(),
	)
	require.NoError(t, err)

	return resp.GetNotes()
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

	taskID := createTaskForNote(t, env, "Task With Notes")

	_, err := client.CreateNote(context.Background(),
		(&dcimv1.CreateNoteRequest_builder{
			EntityType: dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_TASK,
			EntityId:   taskID,
			Body:       "disk swapped",
		}).Build(),
	)
	require.NoError(t, err)

	notes := listTaskNotes(t, env, taskID)
	require.Len(t, notes, 1)

	assert.Equal(t, "disk swapped", notes[0].GetBody())
	assert.Equal(t, callerName, notes[0].GetCreatedBy())
	// The id is what a client joins onto the roster with; the display name alone
	// would collide between users who share a name.
	assert.Equal(t, env.subject, notes[0].GetCreatedById())
}

// A soft-deleted author's still-valid token is refused at the interceptor —
// revoked users must not keep writing, attributed or otherwise.
func TestNoteService_CreateNote_SoftDeletedAuthor(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewNoteServiceClient(env.client(), env.server.URL)

	taskID := createTaskForNote(t, env, "Task Noted After Deletion")

	_, err := env.adminPool.Exec(context.Background(),
		`UPDATE dcim.users SET deleted = now() WHERE id = $1`, env.subject)
	require.NoError(t, err)

	_, err = client.CreateNote(context.Background(),
		(&dcimv1.CreateNoteRequest_builder{
			EntityType: dcimv1.NoteEntityType_NOTE_ENTITY_TYPE_TASK,
			EntityId:   taskID,
			Body:       "written after the author was removed",
		}).Build(),
	)
	requireCode(t, err, connect.CodeUnauthenticated)

	// Straight to the database: the revoked caller can no longer list either.
	var count int
	err = env.adminPool.QueryRow(context.Background(),
		`SELECT count(*) FROM dcim.notes WHERE task_id = $1`, taskID).Scan(&count)
	require.NoError(t, err)
	assert.Zero(t, count, "a revoked caller must not write notes")
}
