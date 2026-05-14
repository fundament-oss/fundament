package dcim

import (
	"fmt"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func taskStatusToProto(s string) dcimv1.TaskStatus {
	switch s {
	case "ready":
		return dcimv1.TaskStatus_TASK_STATUS_READY
	case "in_progress":
		return dcimv1.TaskStatus_TASK_STATUS_IN_PROGRESS
	case "review":
		return dcimv1.TaskStatus_TASK_STATUS_REVIEW
	case "blocked":
		return dcimv1.TaskStatus_TASK_STATUS_BLOCKED
	case "done":
		return dcimv1.TaskStatus_TASK_STATUS_DONE
	default:
		panic("unknown task status: " + s)
	}
}

func taskStatusFromProto(s dcimv1.TaskStatus) string {
	switch s {
	case dcimv1.TaskStatus_TASK_STATUS_READY:
		return "ready"
	case dcimv1.TaskStatus_TASK_STATUS_IN_PROGRESS:
		return "in_progress"
	case dcimv1.TaskStatus_TASK_STATUS_REVIEW:
		return "review"
	case dcimv1.TaskStatus_TASK_STATUS_BLOCKED:
		return "blocked"
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
	case "critical":
		return dcimv1.TaskPriority_TASK_PRIORITY_CRITICAL
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
	case dcimv1.TaskPriority_TASK_PRIORITY_CRITICAL:
		return "critical"
	default:
		panic(fmt.Sprintf("unknown task priority: %d", s))
	}
}

func taskCategoryToProto(s string) dcimv1.TaskCategory {
	switch s {
	case "hardware":
		return dcimv1.TaskCategory_TASK_CATEGORY_HARDWARE
	case "network":
		return dcimv1.TaskCategory_TASK_CATEGORY_NETWORK
	case "cooling":
		return dcimv1.TaskCategory_TASK_CATEGORY_COOLING
	case "power":
		return dcimv1.TaskCategory_TASK_CATEGORY_POWER
	case "security":
		return dcimv1.TaskCategory_TASK_CATEGORY_SECURITY
	case "other":
		return dcimv1.TaskCategory_TASK_CATEGORY_OTHER
	default:
		panic("unknown task category: " + s)
	}
}

func taskCategoryFromProto(s dcimv1.TaskCategory) string {
	switch s {
	case dcimv1.TaskCategory_TASK_CATEGORY_HARDWARE:
		return "hardware"
	case dcimv1.TaskCategory_TASK_CATEGORY_NETWORK:
		return "network"
	case dcimv1.TaskCategory_TASK_CATEGORY_COOLING:
		return "cooling"
	case dcimv1.TaskCategory_TASK_CATEGORY_POWER:
		return "power"
	case dcimv1.TaskCategory_TASK_CATEGORY_SECURITY:
		return "security"
	case dcimv1.TaskCategory_TASK_CATEGORY_OTHER:
		return "other"
	default:
		panic(fmt.Sprintf("unknown task category: %d", s))
	}
}

func taskFromRow(row *db.TaskGetByIDRow) *dcimv1.Task {
	task := dcimv1.Task_builder{
		Id:       row.ID.String(),
		Title:    row.Title,
		Status:   taskStatusToProto(row.Status),
		Priority: taskPriorityToProto(row.Priority),
		Category: taskCategoryToProto(row.Category),
		Created:  timestamppb.New(row.Created.Time),
	}.Build()

	if row.Description.Valid {
		task.SetDescription(row.Description.String)
	}

	if row.AssigneeID.Valid {
		task.SetAssigneeId(row.AssigneeID.String)
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
		Category: taskCategoryToProto(row.Category),
		Created:  timestamppb.New(row.Created.Time),
	}.Build()

	if row.Description.Valid {
		task.SetDescription(row.Description.String)
	}

	if row.AssigneeID.Valid {
		task.SetAssigneeId(row.AssigneeID.String)
	}

	if row.DueDate.Valid {
		task.SetDueDate(timestamppb.New(row.DueDate.Time))
	}

	if row.Location.Valid {
		task.SetLocation(row.Location.String)
	}

	return task
}
