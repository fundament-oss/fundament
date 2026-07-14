package installproxy

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/authz"
)

// Authz enforces OpenFGA can_view on (user, cluster). The user is the
// PluginToken's subject — the user the token was minted for.
type Authz struct {
	Client *authz.Client
}

func (a Authz) CanViewCluster(ctx context.Context, userID, clusterID string) (bool, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return false, fmt.Errorf("parse user id: %w", err)
	}
	cid, err := uuid.Parse(clusterID)
	if err != nil {
		return false, fmt.Errorf("parse cluster id: %w", err)
	}
	ok, err := a.Client.CanViewCluster(ctx, uid, cid)
	if err != nil {
		return false, fmt.Errorf("openfga can_view: %w", err)
	}
	return ok, nil
}
