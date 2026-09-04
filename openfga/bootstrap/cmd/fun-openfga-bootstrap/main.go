// Command fun-openfga-bootstrap brings the OpenFGA datastore to the state this
// release expects: one store with the shipped authorization model in force.
//
// It runs as a Job, once per release, so nothing else ever creates a store.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
	language "github.com/openfga/language/pkg/go/transformer"

	"github.com/fundament-oss/fundament/common/authz"
)

type config struct {
	APIURL    string        `env:"OPENFGA_API_URL,required,notEmpty"`
	StoreName string        `env:"OPENFGA_STORE_NAME,notEmpty" envDefault:"fundament"`
	Timeout   time.Duration `env:"OPENFGA_TIMEOUT" envDefault:"10m"`
}

const pollInterval = 2 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("bootstrap failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var cfg config
	if err := env.Parse(&cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	fga, err := client.NewSdkClient(&client.ClientConfiguration{ApiUrl: cfg.APIURL})
	if err != nil {
		return fmt.Errorf("create OpenFGA client: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	if err := awaitServer(waitCtx, fga, cfg.APIURL); err != nil {
		return err
	}

	storeID, err := ensureStore(ctx, fga, cfg.StoreName)
	if err != nil {
		return err
	}
	modelID, err := ensureModel(ctx, fga, storeID)
	if err != nil {
		return err
	}

	slog.Info("datastore ready", "store", cfg.StoreName, "store_id", storeID, "model_id", modelID)

	return nil
}

// awaitServer blocks until OpenFGA answers a store listing, which needs the
// server reachable through its Service and its schema migrated.
func awaitServer(ctx context.Context, fga *client.OpenFgaClient, apiURL string) error {
	slog.Info("waiting until openfga answers ListStores", "api_url", apiURL)

	started := time.Now()

	for attempt := 1; ; attempt++ {
		if _, err := fga.ListStores(ctx).Execute(); err == nil {
			slog.Info("openfga is answering", "waited", time.Since(started).Round(time.Second))

			return nil
		} else if attempt%15 == 0 {
			slog.Info("still waiting for openfga",
				"api_url", apiURL, "waited", time.Since(started).Round(time.Second), "last_error", err)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("openfga did not answer at %s within %s: %w",
				apiURL, time.Since(started).Round(time.Second), ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// ensureStore returns the id of the store called name, creating it when absent.
//
// Store names are not unique in OpenFGA and the window between listing and
// creating is a few milliseconds wide, so after creating it looks again, adopts
// the oldest match and deletes the one it just made if it lost. Two stores split
// consumers across them with every pod healthy.
func ensureStore(ctx context.Context, fga *client.OpenFgaClient, name string) (string, error) {
	id, err := authz.ResolveStoreID(ctx, fga, name)
	if err == nil {
		slog.Info("using the existing store", "name", name, "store_id", id)

		return id, nil
	}

	if !errors.Is(err, authz.ErrNoStore) {
		return "", fmt.Errorf("look up store %q: %w", name, err)
	}

	slog.Info("creating store", "name", name)

	created, err := fga.CreateStore(ctx).Body(client.ClientCreateStoreRequest{Name: name}).Execute()
	if err != nil {
		return "", fmt.Errorf("create store: %w", err)
	}

	winner, err := authz.ResolveStoreID(ctx, fga, name)
	if err != nil {
		return "", fmt.Errorf("re-read store %q after creating it: %w", name, err)
	}

	if winner != created.Id {
		slog.Warn("lost a creation race, discarding the store just created",
			"discarding", created.Id, "adopting", winner)

		if _, err := fga.DeleteStore(ctx).
			Options(client.ClientDeleteStoreOptions{StoreId: &created.Id}).
			Execute(); err != nil {
			return "", fmt.Errorf("delete duplicate store %s: %w", created.Id, err)
		}
	}

	return winner, nil
}

// ensureModel writes the shipped model when it differs from the store's latest,
// and returns the id of the model now in force.
//
// OpenFGA assigns a ULID to each model it accepts, so every write produces a new
// version with an id nothing outside the server can predict.
func ensureModel(ctx context.Context, fga *client.OpenFgaClient, storeID string) (string, error) {
	want, err := decodeModel(authz.ModelDSL)
	if err != nil {
		return "", err
	}

	// An absent model reads as an empty list, so a 404 here means the store
	// itself went away.
	current, err := fga.ReadLatestAuthorizationModel(ctx).
		Options(client.ClientReadLatestAuthorizationModelOptions{StoreId: &storeID}).
		Execute()
	if err != nil {
		return "", fmt.Errorf("read latest authorization model: %w", err)
	}

	reason := "no authorization model in the store yet"

	if current != nil && current.AuthorizationModel != nil {
		same, err := sameModel(current.AuthorizationModel, want)
		if err != nil {
			return "", err
		}

		if same {
			slog.Info("authorization model unchanged", "model_id", current.AuthorizationModel.Id)

			return current.AuthorizationModel.Id, nil
		}

		reason = "shipped model differs from " + current.AuthorizationModel.Id
	}

	slog.Info("writing authorization model", "reason", reason)

	written, err := fga.WriteAuthorizationModel(ctx).
		Body(client.ClientWriteAuthorizationModelRequest{
			SchemaVersion:   want.SchemaVersion,
			TypeDefinitions: want.TypeDefinitions,
			Conditions:      want.Conditions,
		}).
		Options(client.ClientWriteAuthorizationModelOptions{StoreId: &storeID}).
		Execute()
	if err != nil {
		return "", fmt.Errorf("write authorization model: %w", err)
	}

	return written.AuthorizationModelId, nil
}

func decodeModel(dsl string) (*openfga.AuthorizationModel, error) {
	asJSON, err := language.TransformDSLToJSON(dsl)
	if err != nil {
		return nil, fmt.Errorf("parse model: %w", err)
	}

	var model openfga.AuthorizationModel
	if err := json.Unmarshal([]byte(asJSON), &model); err != nil {
		return nil, fmt.Errorf("decode model: %w", err)
	}

	return &model, nil
}

// sameModel reports whether two models mean the same thing.
//
// The comparison goes through the DSL because the server materialises zero
// values the DSL transform leaves unset — absent metadata comes back as null,
// unset modules as "", omitted user-type lists as [] — so the decoded
// structures never match literally even when the model is unchanged.
func sameModel(current, want *openfga.AuthorizationModel) (bool, error) {
	a, err := canonical(current)
	if err != nil {
		return false, err
	}

	b, err := canonical(want)
	if err != nil {
		return false, err
	}

	return a == b, nil
}

func canonical(model *openfga.AuthorizationModel) (string, error) {
	raw, err := json.Marshal(model)
	if err != nil {
		return "", fmt.Errorf("encode authorization model: %w", err)
	}

	dsl, err := language.TransformJSONStringToDSL(string(raw))
	if err != nil {
		return "", fmt.Errorf("render authorization model: %w", err)
	}

	return *dsl, nil
}
