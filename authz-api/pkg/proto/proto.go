// The action catalogue comes first: buf reads the proto it writes.
//
//go:generate go run ../../../common/authz/gen -input ../../../common/authz/model.fga -actions-lock ../../../common/authz/actions/actions.lock.json -actions-proto v1/actions.proto -actions-go ../../../common/authz/actions/actions_gen.go
//go:generate buf generate
package proto
