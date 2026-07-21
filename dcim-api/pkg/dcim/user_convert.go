package dcim

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

// userToProto builds the wire User. external_ref is deliberately left off the
// wire: it is an identity-provider detail, and callers address users by their
// internal id. email is left off too — see userToProtoWithEmail.
func userToProto(id uuid.UUID, name string) *dcimv1.User {
	return dcimv1.User_builder{
		Id:   id.String(),
		Name: name,
	}.Build()
}

// userToProtoWithEmail is userToProto for the caller reading their own entry,
// where the address is theirs to see. The roster listing does not use this: it
// is readable by every authenticated caller and has no consumer for the email.
func userToProtoWithEmail(id uuid.UUID, name string, email pgtype.Text) *dcimv1.User {
	user := userToProto(id, name)

	if email.Valid {
		user.SetEmail(email.String)
	}

	return user
}
