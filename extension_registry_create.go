package vale

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/arcgolabs/observabilityx"
	"github.com/arcgolabs/vale/certstore"
	"github.com/arcgolabs/vale/provider"
	"github.com/samber/oops"
)

func (r *Registry) CreateConfigProviderAs[P provider.ConfigProvider](
	ctx context.Context,
	spec ProviderSpec,
) (P, error) {
	var zero P
	created, err := r.CreateConfigProvider(ctx, spec)
	if err != nil {
		return zero, err
	}
	typed, ok := created.(P)
	if !ok {
		expectedType := fmt.Sprintf("%T", zero)
		actualType := fmt.Sprintf("%T", created)
		message := fmt.Sprintf(
			"config provider has unexpected type: expected %s, got %s",
			expectedType,
			actualType,
		)
		return zero, oops.
			In("vale").
			With(
				"type", spec.Type,
				"name", spec.Name,
				"expected_type", expectedType,
				"actual_type", actualType,
			).
			New(message)
	}
	return typed, nil
}

func (r *Registry) CreateSnapshotProviderAs[P provider.SnapshotProvider](
	ctx context.Context,
	spec ProviderSpec,
) (P, error) {
	var zero P
	created, err := r.CreateSnapshotProvider(ctx, spec)
	if err != nil {
		return zero, err
	}
	typed, ok := created.(P)
	if !ok {
		expectedType := fmt.Sprintf("%T", zero)
		actualType := fmt.Sprintf("%T", created)
		message := fmt.Sprintf(
			"snapshot provider has unexpected type: expected %s, got %s",
			expectedType,
			actualType,
		)
		return zero, oops.
			In("vale").
			With(
				"type", spec.Type,
				"name", spec.Name,
				"expected_type", expectedType,
				"actual_type", actualType,
			).
			New(message)
	}
	return typed, nil
}

func (r *Registry) CreateCertificateStorageAs[S certstore.Storage](
	ctx context.Context,
	name string,
) (S, error) {
	var zero S
	created, err := r.CreateCertificateStorage(ctx, name)
	if err != nil {
		return zero, err
	}
	typed, ok := created.(S)
	if !ok {
		expectedType := fmt.Sprintf("%T", zero)
		actualType := fmt.Sprintf("%T", created)
		message := fmt.Sprintf(
			"certificate storage has unexpected type: expected %s, got %s",
			expectedType,
			actualType,
		)
		return zero, oops.
			In("vale").
			With(
				"name", normalizeRegistryName(name),
				"expected_type", expectedType,
				"actual_type", actualType,
			).
			New(message)
	}
	return typed, nil
}

func (r *Registry) CreateObservabilityAs[O observabilityx.Observability](
	name string,
	logger *slog.Logger,
) (O, error) {
	var zero O
	created, err := r.CreateObservability(name, logger)
	if err != nil {
		return zero, err
	}
	typed, ok := created.(O)
	if !ok {
		expectedType := fmt.Sprintf("%T", zero)
		actualType := fmt.Sprintf("%T", created)
		message := fmt.Sprintf(
			"observability has unexpected type: expected %s, got %s",
			expectedType,
			actualType,
		)
		return zero, oops.
			In("vale").
			With(
				"name", normalizeRegistryName(name),
				"expected_type", expectedType,
				"actual_type", actualType,
			).
			New(message)
	}
	return typed, nil
}
