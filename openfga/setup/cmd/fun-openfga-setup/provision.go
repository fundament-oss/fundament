package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
	language "github.com/openfga/language/pkg/go/transformer"

	"github.com/fundament-oss/fundament/openfga/model"
)

// Status is what the provisioner publishes once the store is ready. Consumers
// compare Generation against their own release to know the datastore is theirs.
type Status struct {
	Generation string `json:"generation"`
	Store      string `json:"store"`
	StoreID    string `json:"id"`
}

// ProvisionConfig configures the provisioning role.
type ProvisionConfig struct {
	APIURL     string        `env:"OPENFGA_API_URL,required,notEmpty"`
	StoreName  string        `env:"OPENFGA_STORE_NAME,notEmpty" envDefault:"fundament"`
	Generation string        `env:"OPENFGA_GENERATION,required,notEmpty"`
	StatusAddr string        `env:"OPENFGA_STATUS_ADDR,notEmpty" envDefault:":8099"`
	Timeout    time.Duration `env:"OPENFGA_TIMEOUT" envDefault:"5m"`
}

const pollInterval = 2 * time.Second

// Provision creates the store if it is absent, brings its authorization model up
// to date, and then serves the status until it is shut down.
func Provision(ctx context.Context, cfg *ProvisionConfig) error {
	// The DSL is what humans edit; the API takes JSON.
	modelJSON, err := language.TransformDSLToJSON(model.DSL)
	if err != nil {
		return fmt.Errorf("parse model: %w", err)
	}

	var want openfga.AuthorizationModel
	if err := json.Unmarshal([]byte(modelJSON), &want); err != nil {
		return fmt.Errorf("decode model: %w", err)
	}

	fga, err := client.NewSdkClient(&client.ClientConfiguration{ApiUrl: cfg.APIURL})
	if err != nil {
		return fmt.Errorf("create OpenFGA client: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	if err := awaitServer(waitCtx, fga); err != nil {
		return err
	}

	storeID, err := ensureStore(ctx, fga, cfg.StoreName)
	if err != nil {
		return err
	}

	if err := ensureModel(ctx, fga, storeID, &want); err != nil {
		return err
	}

	slog.Info("datastore provisioned", "store", cfg.StoreName, "id", storeID, "generation", cfg.Generation)

	return serveStatus(ctx, cfg.StatusAddr, Status{
		Generation: cfg.Generation,
		Store:      cfg.StoreName,
		StoreID:    storeID,
	})
}

// awaitServer blocks until OpenFGA can answer a store listing. Its /healthz
// reports 200 with the datastore destroyed, so it says nothing useful here.
func awaitServer(ctx context.Context, fga *client.OpenFgaClient) error {
	for {
		if _, err := fga.ListStores(ctx).Execute(); err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("openfga did not become reachable: %w", ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}
