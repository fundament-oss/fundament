package provider

import (
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// timestampValue formats an API timestamp as RFC 3339, the format of every
// created attribute; null when the API sent none.
func timestampValue(ts *timestamppb.Timestamp) types.String {
	if ts.CheckValid() != nil {
		return types.StringNull()
	}
	return types.StringValue(ts.AsTime().Format(time.RFC3339))
}
