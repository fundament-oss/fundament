package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

func TestComputeReplication(t *testing.T) {
	tests := []struct {
		name       string
		requested  *int32
		nodeCount  int
		wantRepl   int
		wantDomain string
		wantMsgHas string
	}{
		{"derived three nodes", nil, 3, 3, "host", ""},
		{"derived five nodes caps at 3", nil, 5, 3, "host", ""},
		{"derived two nodes", nil, 2, 2, "host", ""},
		{"derived one node uses osd domain", nil, 1, 1, "osd", ""},
		{"derived zero nodes", nil, 0, 1, "osd", ""},
		{"explicit 3 on 2 nodes clamps", ptr.To[int32](3), 2, 2, "host", "clamped"},
		{"explicit 3 on 3 nodes", ptr.To[int32](3), 3, 3, "host", ""},
		{"explicit 2 on 1 node clamps to osd", ptr.To[int32](2), 1, 1, "osd", "clamped"},
		{"explicit 1", ptr.To[int32](1), 3, 1, "osd", ""},
		// The CRD minimum keeps these out of the API. An invalid value must
		// not silently mean "no replication".
		{"zero is derived", ptr.To[int32](0), 2, 2, "host", "deriving"},
		{"negative is derived", ptr.To[int32](-1), 3, 3, "host", "deriving"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repl, domain, msg := ComputeReplication(tt.requested, tt.nodeCount)
			assert.Equal(t, tt.wantRepl, repl)
			assert.Equal(t, tt.wantDomain, domain)
			if tt.wantMsgHas == "" {
				assert.Empty(t, msg)
			} else {
				assert.Contains(t, msg, tt.wantMsgHas)
			}
		})
	}
}
