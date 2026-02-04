package client

import (
	"context"

	"connectrpc.com/connect"

	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
)

// GetUserInfo gets the current user's information.
func (c *Client) GetUserInfo(ctx context.Context) (*authnv1.User, error) {
	resp, err := c.authn().GetUserInfo(ctx, connect.NewRequest(&authnv1.GetUserInfoRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.User, nil
}
