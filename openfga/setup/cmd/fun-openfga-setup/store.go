package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
	language "github.com/openfga/language/pkg/go/transformer"
)

// ensureStore returns the id of the store called name, creating it when absent.
//
// Names are not unique in OpenFGA, so when several match it picks the oldest, the
// same rule common/authz applies. Anything else would let consumers and this
// provisioner disagree about which store is current.
func ensureStore(ctx context.Context, fga *client.OpenFgaClient, name string) (string, error) {
	matches, err := listStoresNamed(ctx, fga, name)
	if err != nil {
		return "", err
	}

	if len(matches) > 0 {
		if len(matches) > 1 {
			ids := make([]string, 0, len(matches))
			for _, m := range matches {
				ids = append(ids, m.Id)
			}

			slog.Warn("multiple stores share one name, using the oldest",
				"name", name, "using", matches[0].Id, "found", strings.Join(ids, ","))
		}

		slog.Info("using the existing store", "name", name, "id", matches[0].Id)

		return matches[0].Id, nil
	}

	slog.Info("creating store", "name", name)

	created, err := fga.CreateStore(ctx).Body(client.ClientCreateStoreRequest{Name: name}).Execute()
	if err != nil {
		return "", fmt.Errorf("create store: %w", err)
	}

	return created.Id, nil
}

// listStoresNamed returns every live store called name, following pagination.
func listStoresNamed(ctx context.Context, fga *client.OpenFgaClient, name string) ([]openfga.Store, error) {
	var (
		matches []openfga.Store
		token   string
	)

	for {
		opts := client.ClientListStoresOptions{Name: &name}
		if token != "" {
			opts.ContinuationToken = &token
		}

		resp, err := fga.ListStores(ctx).Options(opts).Execute()
		if err != nil {
			return nil, fmt.Errorf("list stores: %w", err)
		}

		for _, store := range resp.GetStores() {
			if store.Name == name && store.DeletedAt == nil {
				matches = append(matches, store)
			}
		}

		if token = resp.GetContinuationToken(); token == "" {
			break
		}
	}

	slices.SortFunc(matches, func(a, b openfga.Store) int {
		if byAge := a.CreatedAt.Compare(b.CreatedAt); byAge != 0 {
			return byAge
		}

		return strings.Compare(a.Id, b.Id)
	})

	return matches, nil
}

// ensureModel writes the authorization model only when the store's latest one
// differs. OpenFGA never dedupes: every write appends a version with a fresh id,
// and since no consumer pins a model id they would all silently follow it.
func ensureModel(ctx context.Context, fga *client.OpenFgaClient, storeID string, want *openfga.AuthorizationModel) error {
	current, err := fga.ReadLatestAuthorizationModel(ctx).Options(
		client.ClientReadLatestAuthorizationModelOptions{StoreId: &storeID},
	).Execute()

	var notFound openfga.FgaApiNotFoundError

	// Carried into the write log: an operator seeing a model written wants to know
	// whether the store had none or the shipped model actually changed.
	reason := "no authorization model in the store yet"

	switch {
	// A read that failed for any other reason must not pass for "no model yet":
	// writing then appends a version every consumer immediately evaluates against.
	case err != nil && !errors.As(err, &notFound):
		return fmt.Errorf("read latest authorization model: %w", err)

	case err == nil && current.AuthorizationModel != nil:
		same, err := sameModel(current.AuthorizationModel, want)
		if err != nil {
			return err
		}

		if same {
			slog.Info("authorization model unchanged", "model_id", current.AuthorizationModel.Id)

			return nil
		}

		reason = "shipped model differs from the store's latest, " + current.AuthorizationModel.Id
	}

	slog.Info("writing authorization model", "reason", reason)

	body := client.ClientWriteAuthorizationModelRequest{
		SchemaVersion:   want.SchemaVersion,
		TypeDefinitions: want.TypeDefinitions,
		Conditions:      want.Conditions,
	}

	written, err := fga.WriteAuthorizationModel(ctx).
		Body(body).
		Options(client.ClientWriteAuthorizationModelOptions{StoreId: &storeID}).
		Execute()
	if err != nil {
		return fmt.Errorf("write authorization model: %w", err)
	}

	slog.Info("authorization model written", "model_id", written.AuthorizationModelId)

	return nil
}

// sameModel reports whether the store's latest model means the same as the one the
// chart ships. The comparison goes through the DSL because the server materialises
// zero values the DSL transform leaves unset — absent metadata comes back as null,
// unset modules as "", omitted user-type lists as [] — so the two decoded structures
// never match literally even when the model is unchanged.
func sameModel(current, want *openfga.AuthorizationModel) (bool, error) {
	a, err := canonicalModel(current)
	if err != nil {
		return false, err
	}

	b, err := canonicalModel(want)
	if err != nil {
		return false, err
	}

	return a == b, nil
}

func canonicalModel(model *openfga.AuthorizationModel) (string, error) {
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
