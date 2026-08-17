package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeReplication(t *testing.T) {
	tests := []struct {
		name       string
		requested  string
		nodeCount  int
		wantRepl   int
		wantDomain string
		wantMsgHas string
	}{
		{"auto three nodes", "auto", 3, 3, "host", ""},
		{"auto five nodes caps at 3", "auto", 5, 3, "host", ""},
		{"auto two nodes", "auto", 2, 2, "host", ""},
		{"auto one node uses osd domain", "auto", 1, 1, "osd", ""},
		{"auto zero nodes", "auto", 0, 1, "osd", ""},
		{"explicit 3 on 2 nodes clamps", "3", 2, 2, "host", "clamped"},
		{"explicit 3 on 3 nodes", "3", 3, 3, "host", ""},
		{"explicit 2 on 1 node clamps to osd", "2", 1, 1, "osd", "clamped"},
		{"explicit 1", "1", 3, 1, "osd", ""},
		{"empty is auto", "", 3, 3, "host", ""},
		// The CRD enum keeps these out of the API today. If it ever widens, an
		// unrecognised value must not silently mean "no replication".
		{"garbage falls back to auto", "three", 3, 3, "host", "using auto"},
		{"zero falls back to auto", "0", 2, 2, "host", "using auto"},
		{"negative falls back to auto", "-1", 3, 3, "host", "using auto"},
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
