package authz

import _ "embed"

// ModelDSL is the authorization model this build evaluates against. The
// provisioner that writes it and the generated types come from this one file.
//
//go:embed model.fga
var ModelDSL string
