//go:generate go run ./gen/main.go -input ../../openfga/model.fga -output types_gen.go
//go:generate go fmt types_gen.go
package authz
