package models

type NamespaceCreate struct {
	Name string `validate:"required,min=1,max=63,dns1123label"`
}
