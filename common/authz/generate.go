//go:generate go run ./gen/main.go -input model.fga -output types_gen.go
//go:generate go fmt types_gen.go
package authz
