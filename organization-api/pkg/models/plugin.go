package models

type PluginCreate struct {
	PluginID string `validate:"required,min=1,max=255"`
}
