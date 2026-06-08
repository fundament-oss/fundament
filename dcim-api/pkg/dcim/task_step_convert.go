package dcim

import (
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func taskStepFromListRow(row *db.TaskStepListRow) *dcimv1.TaskStep {
	step := dcimv1.TaskStep_builder{
		Id:        row.ID.String(),
		TaskId:    row.TaskID.String(),
		Title:     row.Title,
		Ordinal:   row.Ordinal,
		Completed: row.Completed,
		Created:   timestamppb.New(row.Created.Time),
	}.Build()

	if row.Description.Valid {
		step.SetDescription(row.Description.String)
	}

	return step
}
