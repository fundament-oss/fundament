// Package model holds the OpenFGA authorization model the platform runs on.
package model

import _ "embed"

// DSL is the authorization model the provisioner writes to the store.
//
// It is embedded rather than mounted from a ConfigMap: a ConfigMap change leaves
// the pod template untouched, so nothing reruns the provisioner and the edit
// takes effect whenever the pod happens to restart next. Embedding ties the model
// to an image tag, which is what makes a change roll out.
//
//go:embed model.fga
var DSL string
