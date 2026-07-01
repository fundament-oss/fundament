package dcim

import (
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func userFromRow(row *db.UserListRow) *dcimv1.User {
	user := dcimv1.User_builder{
		Id:   row.ID.String(),
		Name: row.Name,
	}.Build()

	if row.Email.Valid {
		user.SetEmail(row.Email.String)
	}

	return user
}
