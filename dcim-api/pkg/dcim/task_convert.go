package dcim

import (
	"fmt"

	"github.com/google/uuid"

	"google.golang.org/protobuf/types/known/timestamppb"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func taskStatusToProto(s string) dcimv1.TaskStatus {
	switch s {
	case "todo":
		return dcimv1.TaskStatus_TASK_STATUS_TODO
	case "doing":
		return dcimv1.TaskStatus_TASK_STATUS_DOING
	case "done":
		return dcimv1.TaskStatus_TASK_STATUS_DONE
	default:
		panic("unknown task status: " + s)
	}
}

func taskStatusFromProto(s dcimv1.TaskStatus) string {
	switch s {
	case dcimv1.TaskStatus_TASK_STATUS_TODO:
		return "todo"
	case dcimv1.TaskStatus_TASK_STATUS_DOING:
		return "doing"
	case dcimv1.TaskStatus_TASK_STATUS_DONE:
		return "done"
	default:
		panic(fmt.Sprintf("unknown task status: %d", s))
	}
}

func taskPriorityToProto(s string) dcimv1.TaskPriority {
	switch s {
	case "low":
		return dcimv1.TaskPriority_TASK_PRIORITY_LOW
	case "medium":
		return dcimv1.TaskPriority_TASK_PRIORITY_MEDIUM
	case "high":
		return dcimv1.TaskPriority_TASK_PRIORITY_HIGH
	case "urgent":
		return dcimv1.TaskPriority_TASK_PRIORITY_URGENT
	// Nobody has prioritized this one yet.
	case "none":
		return dcimv1.TaskPriority_TASK_PRIORITY_UNSPECIFIED
	default:
		panic("unknown task priority: " + s)
	}
}

func taskPriorityFromProto(s dcimv1.TaskPriority) string {
	switch s {
	case dcimv1.TaskPriority_TASK_PRIORITY_LOW:
		return "low"
	case dcimv1.TaskPriority_TASK_PRIORITY_MEDIUM:
		return "medium"
	case dcimv1.TaskPriority_TASK_PRIORITY_HIGH:
		return "high"
	case dcimv1.TaskPriority_TASK_PRIORITY_URGENT:
		return "urgent"
	case dcimv1.TaskPriority_TASK_PRIORITY_UNSPECIFIED:
		return "none"
	default:
		panic(fmt.Sprintf("unknown task priority: %d", s))
	}
}

func taskFromRow(row *db.TaskGetByIDRow) *dcimv1.Task {
	task := dcimv1.Task_builder{
		Id:       row.ID.String(),
		Title:    row.Title,
		Status:   taskStatusToProto(row.Status),
		Priority: taskPriorityToProto(row.Priority),
		Tags:     row.Tags,
		Created:  timestamppb.New(row.Created.Time),
	}.Build()

	if row.BlockedReason.Valid {
		task.SetBlockedReason(row.BlockedReason.String)
	}

	if row.Description.Valid {
		task.SetDescription(row.Description.String)
	}

	if row.AssigneeID.Valid {
		task.SetAssigneeId(uuid.UUID(row.AssigneeID.Bytes).String())
	}

	if row.DueDate.Valid {
		task.SetDueDate(timestamppb.New(row.DueDate.Time))
	}

	if row.Location.Valid {
		task.SetLocation(row.Location.String)
	}

	return task
}

func taskFromListRow(row *db.TaskListRow) *dcimv1.Task {
	task := dcimv1.Task_builder{
		Id:       row.ID.String(),
		Title:    row.Title,
		Status:   taskStatusToProto(row.Status),
		Priority: taskPriorityToProto(row.Priority),
		Tags:     row.Tags,
		Created:  timestamppb.New(row.Created.Time),
	}.Build()

	if row.BlockedReason.Valid {
		task.SetBlockedReason(row.BlockedReason.String)
	}

	if row.Description.Valid {
		task.SetDescription(row.Description.String)
	}

	if row.AssigneeID.Valid {
		task.SetAssigneeId(uuid.UUID(row.AssigneeID.Bytes).String())
	}

	if row.DueDate.Valid {
		task.SetDueDate(timestamppb.New(row.DueDate.Time))
	}

	if row.Location.Valid {
		task.SetLocation(row.Location.String)
	}

	return task
}
