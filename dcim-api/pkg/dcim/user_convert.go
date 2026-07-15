package dcim

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

// userToProto builds the wire User. external_ref is deliberately left off the
// wire: it is an identity-provider detail, and callers address users by their
// internal id.
func userToProto(id uuid.UUID, name string, email pgtype.Text) *dcimv1.User {
	user := dcimv1.User_builder{
		Id:   id.String(),
		Name: name,
	}.Build()

	if email.Valid {
		user.SetEmail(email.String)
	}

	return user
}
