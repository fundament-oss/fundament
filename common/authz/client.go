package authz

import (
	"context"
	"fmt"

	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
)

// Config holds configuration for the OpenFGA client.
type Config struct {
	APIURL               string `env:"OPENFGA_API_URL,required,notEmpty"`
	StoreID              string `env:"OPENFGA_STORE_ID"`
	AuthorizationModelID string `env:"OPENFGA_AUTHORIZATION_MODEL_ID"`
}

// Client wraps the OpenFGA SDK client with convenience methods.
type Client struct {
	fga *client.OpenFgaClient
}

// New creates a new OpenFGA authorization client.
func New(cfg Config) (*Client, error) {
	fgaClient, err := client.NewSdkClient(&client.ClientConfiguration{
		ApiUrl:               cfg.APIURL,
		StoreId:              cfg.StoreID,
		AuthorizationModelId: cfg.AuthorizationModelID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenFGA client: %w", err)
	}

	return &Client{fga: fgaClient}, nil
}

// Check performs an authorization check.
// Returns true if the user has the specified relation on the object.
func (c *Client) Check(ctx context.Context, user, relation, object string) (bool, error) {
	resp, err := c.fga.Check(ctx).Body(client.ClientCheckRequest{
		User:     user,
		Relation: relation,
		Object:   object,
	}).Execute()
	if err != nil {
		return false, fmt.Errorf("OpenFGA check failed: %w", err)
	}

	if resp.Allowed == nil {
		return false, nil
	}

	return *resp.Allowed, nil
}

// WriteTuples writes relationship tuples to OpenFGA.
func (c *Client) WriteTuples(ctx context.Context, tuples ...openfga.TupleKey) error {
	if len(tuples) == 0 {
		return nil
	}

	_, err := c.fga.WriteTuples(ctx).Body(tuples).Execute()
	if err != nil {
		return fmt.Errorf("OpenFGA write tuples failed: %w", err)
	}

	return nil
}

// DeleteTuples removes relationship tuples from OpenFGA.
func (c *Client) DeleteTuples(ctx context.Context, tuples ...openfga.TupleKeyWithoutCondition) error {
	if len(tuples) == 0 {
		return nil
	}

	_, err := c.fga.DeleteTuples(ctx).Body(tuples).Execute()
	if err != nil {
		return fmt.Errorf("OpenFGA delete tuples failed: %w", err)
	}

	return nil
}

// Tuple creates a TupleKey for writing.
func Tuple(user, relation, object string) openfga.TupleKey {
	return openfga.TupleKey{
		User:     user,
		Relation: relation,
		Object:   object,
	}
}

// TupleDelete creates a TupleKeyWithoutCondition for deletion.
func TupleDelete(user, relation, object string) openfga.TupleKeyWithoutCondition {
	return openfga.TupleKeyWithoutCondition{
		User:     user,
		Relation: relation,
		Object:   object,
	}
}
