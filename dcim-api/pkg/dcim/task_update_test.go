package dcim_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1/dcimv1connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// dueDate is the due date the fixtures below start out with. Postgres stores
// timestamptz at microsecond precision, so keep the fixture value coarse enough
// to survive the round trip unchanged.
var dueDate = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// createTaskFixture creates a task with every nullable column populated, so the
// update tests can assert on clearing as well as overwriting.
func createTaskFixture(t *testing.T, env *testEnv, title, assigneeID string) string {
	t.Helper()

	description := "fixture description"
	location := "Room A"

	return createTask(t, env, (&dcimv1.CreateTaskRequest_builder{
		Title:       title,
		Description: &description,
		Status:      dcimv1.TaskStatus_TASK_STATUS_READY,
		Priority:    dcimv1.TaskPriority_TASK_PRIORITY_LOW,
		Category:    dcimv1.TaskCategory_TASK_CATEGORY_HARDWARE,
		AssigneeId:  &assigneeID,
		DueDate:     timestamppb.New(dueDate),
		Location:    &location,
	}).Build())
}

func getTask(t *testing.T, env *testEnv, taskID string) *dcimv1.Task {
	t.Helper()

	client := dcimv1connect.NewTaskServiceClient(env.client(), env.server.URL)

	resp, err := client.GetTask(context.Background(), connect.NewRequest(
		(&dcimv1.GetTaskRequest_builder{Id: taskID}).Build(),
	))
	require.NoError(t, err)

	task := resp.Msg.GetTask()
	require.NotNil(t, task)

	return task
}

func TestTaskService_UpdateTask_HappyFlow(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewTaskServiceClient(env.client(), env.server.URL)

	assigneeID := createUser(t, env, "Update Assignee", "assignee@example.com", "")
	newAssigneeID := createUser(t, env, "Update Reassignee", "reassignee@example.com", "")
	taskID := createTaskFixture(t, env, "Task Before Update", assigneeID)

	newTitle := "Task After Update"
	newDescription := "updated description"
	newStatus := dcimv1.TaskStatus_TASK_STATUS_IN_PROGRESS
	newPriority := dcimv1.TaskPriority_TASK_PRIORITY_CRITICAL
	newCategory := dcimv1.TaskCategory_TASK_CATEGORY_NETWORK
	newLocation := "Room B"
	newDueDate := dueDate.Add(48 * time.Hour)

	_, err := client.UpdateTask(context.Background(), connect.NewRequest(
		(&dcimv1.UpdateTaskRequest_builder{
			Id:          taskID,
			Title:       &newTitle,
			Description: &newDescription,
			Status:      &newStatus,
			Priority:    &newPriority,
			Category:    &newCategory,
			AssigneeId:  &newAssigneeID,
			DueDate:     timestamppb.New(newDueDate),
			Location:    &newLocation,
		}).Build(),
	))
	require.NoError(t, err)

	task := getTask(t, env, taskID)

	assert.Equal(t, newTitle, task.GetTitle())
	assert.Equal(t, newDescription, task.GetDescription())
	assert.Equal(t, newStatus, task.GetStatus())
	assert.Equal(t, newPriority, task.GetPriority())
	assert.Equal(t, newCategory, task.GetCategory())
	assert.Equal(t, newAssigneeID, task.GetAssigneeId())
	assert.Equal(t, newLocation, task.GetLocation())
	assert.True(t, newDueDate.Equal(task.GetDueDate().AsTime()))
}

// A field left unset must not be touched. This is the half of the tri-state
// that a naive "assign every param" refactor would silently break, wiping
// columns the caller never mentioned.
func TestTaskService_UpdateTask_UnsetFieldsAreKept(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewTaskServiceClient(env.client(), env.server.URL)

	assigneeID := createUser(t, env, "Keep Assignee", "keep@example.com", "")
	taskID := createTaskFixture(t, env, "Task Keeping Fields", assigneeID)

	// Patch only the title; everything else must survive untouched.
	newTitle := "Only The Title Changed"
	_, err := client.UpdateTask(context.Background(), connect.NewRequest(
		(&dcimv1.UpdateTaskRequest_builder{
			Id:    taskID,
			Title: &newTitle,
		}).Build(),
	))
	require.NoError(t, err)

	task := getTask(t, env, taskID)

	assert.Equal(t, newTitle, task.GetTitle())
	assert.Equal(t, "fixture description", task.GetDescription())
	assert.Equal(t, dcimv1.TaskStatus_TASK_STATUS_READY, task.GetStatus())
	assert.Equal(t, dcimv1.TaskPriority_TASK_PRIORITY_LOW, task.GetPriority())
	assert.Equal(t, dcimv1.TaskCategory_TASK_CATEGORY_HARDWARE, task.GetCategory())
	assert.Equal(t, assigneeID, task.GetAssigneeId())
	assert.Equal(t, "Room A", task.GetLocation())
	assert.True(t, task.HasDueDate())
	assert.True(t, dueDate.Equal(task.GetDueDate().AsTime()))
}

// The nullable columns clear when the caller explicitly sets the "empty"
// sentinel: an empty string, or the epoch timestamp for due_date.
func TestTaskService_UpdateTask_ClearsNullableFields(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewTaskServiceClient(env.client(), env.server.URL)

	assigneeID := createUser(t, env, "Clear Assignee", "clear@example.com", "")
	taskID := createTaskFixture(t, env, "Task Clearing Fields", assigneeID)

	emptyAssignee := ""
	emptyLocation := ""
	_, err := client.UpdateTask(context.Background(), connect.NewRequest(
		(&dcimv1.UpdateTaskRequest_builder{
			Id:         taskID,
			AssigneeId: &emptyAssignee,
			DueDate:    timestamppb.New(time.Unix(0, 0).UTC()),
			Location:   &emptyLocation,
		}).Build(),
	))
	require.NoError(t, err)

	task := getTask(t, env, taskID)

	assert.False(t, task.HasAssigneeId())
	assert.False(t, task.HasDueDate())
	assert.Empty(t, task.GetLocation())

	// Clearing the nullable columns must not disturb the rest of the row.
	assert.Equal(t, "Task Clearing Fields", task.GetTitle())
	assert.Equal(t, "fixture description", task.GetDescription())
	assert.Equal(t, dcimv1.TaskStatus_TASK_STATUS_READY, task.GetStatus())
}

// Clearing is driven by the sentinel, not by the field merely being set: a
// non-empty value on an already-populated column overwrites rather than clears.
func TestTaskService_UpdateTask_ClearIsIndependentPerField(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewTaskServiceClient(env.client(), env.server.URL)

	assigneeID := createUser(t, env, "Mixed Assignee", "mixed@example.com", "")
	taskID := createTaskFixture(t, env, "Task Mixing Clear And Set", assigneeID)

	// Clear the assignee while overwriting the location in the same request.
	emptyAssignee := ""
	newLocation := "Room C"
	_, err := client.UpdateTask(context.Background(), connect.NewRequest(
		(&dcimv1.UpdateTaskRequest_builder{
			Id:         taskID,
			AssigneeId: &emptyAssignee,
			Location:   &newLocation,
		}).Build(),
	))
	require.NoError(t, err)

	task := getTask(t, env, taskID)

	assert.False(t, task.HasAssigneeId())
	assert.Equal(t, newLocation, task.GetLocation())
	assert.True(t, task.HasDueDate(), "an unset due_date must not be cleared by a sibling clear")
}

// A well-formed assignee id that matches no directory entry violates
// dcim_tasks_fk_assignee. That is a caller mistake, not a server fault, so it
// must map onto NotFound rather than leaking out as a 500 — the admin board
// caches the roster, so it can send an id for a user deleted since page load.
func TestTaskService_UpdateTask_UnknownAssignee(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewTaskServiceClient(env.client(), env.server.URL)

	assigneeID := createUser(t, env, "Real Assignee", "real@example.com", "")
	taskID := createTaskFixture(t, env, "Task With Unknown Assignee", assigneeID)

	_, err := client.UpdateTask(context.Background(), connect.NewRequest(
		(&dcimv1.UpdateTaskRequest_builder{
			Id:         taskID,
			AssigneeId: ptr(validUUID),
		}).Build(),
	))
	requireCode(t, err, connect.CodeNotFound)
}

func TestTaskService_CreateTask_UnknownAssignee(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewTaskServiceClient(env.client(), env.server.URL)

	_, err := client.CreateTask(context.Background(), connect.NewRequest(
		(&dcimv1.CreateTaskRequest_builder{
			Title:      "Task For A Ghost",
			Status:     dcimv1.TaskStatus_TASK_STATUS_READY,
			Priority:   dcimv1.TaskPriority_TASK_PRIORITY_LOW,
			Category:   dcimv1.TaskCategory_TASK_CATEGORY_HARDWARE,
			AssigneeId: ptr(validUUID),
		}).Build(),
	))
	requireCode(t, err, connect.CodeNotFound)
}

func TestTaskService_UpdateTask_Errors(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewTaskServiceClient(env.client(), env.server.URL)

	tests := []struct {
		name string
		req  *dcimv1.UpdateTaskRequest
		want connect.Code
	}{
		{
			"empty_id",
			(&dcimv1.UpdateTaskRequest_builder{Id: ""}).Build(),
			connect.CodeInvalidArgument,
		},
		{
			"invalid_uuid",
			(&dcimv1.UpdateTaskRequest_builder{Id: invalidUUID}).Build(),
			connect.CodeInvalidArgument,
		},
		{
			"not_found",
			(&dcimv1.UpdateTaskRequest_builder{Id: validUUID}).Build(),
			connect.CodeNotFound,
		},
		{
			"invalid_assignee_id",
			(&dcimv1.UpdateTaskRequest_builder{
				Id:         validUUID,
				AssigneeId: ptr(invalidUUID),
			}).Build(),
			connect.CodeInvalidArgument,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := client.UpdateTask(context.Background(), connect.NewRequest(tc.req))
			requireCode(t, err, tc.want)
		})
	}
}

func ptr[T any](v T) *T { return &v }
