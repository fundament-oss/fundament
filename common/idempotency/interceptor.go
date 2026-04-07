package idempotency

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

const (
	// HeaderIdempotencyKey is the request header carrying the client-provided idempotency key.
	HeaderIdempotencyKey = "Idempotency-Key"
	// HeaderIdempotencyStatus is the response header indicating processing status.
	HeaderIdempotencyStatus = "Idempotency-Status"
)

// UserIDExtractor extracts the authenticated user ID from context.
type UserIDExtractor func(ctx context.Context) (uuid.UUID, bool)

// NewInterceptor returns a Connect unary interceptor that provides idempotency
// for Create operations. It caches successful responses and returns them on
// replay, along with the current processing status resolved per procedure.
func NewInterceptor(
	logger *slog.Logger,
	store *Store,
	userIDExtractor UserIDExtractor,
	procedures map[string]Procedure,
) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		if store == nil {
			return next
		}

		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			procedure := req.Spec().Procedure

			// Only handle procedures we have a config for.
			proc, ok := procedures[procedure]
			if !ok {
				return next(ctx, req)
			}

			// Extract idempotency key from header.
			keyStr := req.Header().Get(HeaderIdempotencyKey)
			if keyStr == "" {
				return next(ctx, req)
			}

			logger.DebugContext(ctx, "idempotency key received",
				"procedure", procedure,
				"idempotency_key", keyStr,
			)

			idempotencyKey, err := uuid.Parse(keyStr)
			if err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid %s header: %w", HeaderIdempotencyKey, err))
			}

			userID, ok := userIDExtractor(ctx)
			if !ok {
				logger.DebugContext(ctx, "no user in context, skipping idempotency",
					"procedure", procedure,
				)
				return next(ctx, req)
			}

			reqHash, err := hashRequest(req)
			if err != nil {
				logger.WarnContext(ctx, "failed to hash request, skipping idempotency",
					"error", err,
					"procedure", procedure,
				)
				return next(ctx, req)
			}

			// Try to find a cached response.
			cached, err := store.Lookup(ctx, idempotencyKey, userID)
			if err != nil {
				logger.WarnContext(ctx, "idempotency lookup failed, proceeding without cache",
					"error", err,
					"procedure", procedure,
				)
				return next(ctx, req)
			}

			if cached != nil {
				if cached.ResponseBytes != nil {
					logger.DebugContext(ctx, "idempotency cache hit, replaying response",
						"procedure", procedure,
						"idempotency_key", idempotencyKey,
						"resource_id", cached.ResourceID,
					)
					return handleReplay(ctx, logger, cached, reqHash, procedure, proc)
				}

				// Reservation exists without a response — still in progress.
				logger.DebugContext(ctx, "idempotency reservation in progress",
					"procedure", procedure,
					"idempotency_key", idempotencyKey,
				)
				return nil, connect.NewError(connect.CodeAborted,
					fmt.Errorf("a request with this idempotency key is already being processed"))
			}

			// Key not found — reserve it before executing the handler.
			reserved, err := store.Reserve(ctx, ReserveParams{
				IdempotencyKey: idempotencyKey,
				UserID:         userID,
				Procedure:      procedure,
				RequestHash:    reqHash,
			})
			if err != nil {
				logger.WarnContext(ctx, "failed to reserve idempotency key, proceeding without cache",
					"error", err,
					"procedure", procedure,
				)
				return next(ctx, req)
			}

			if !reserved {
				// Race: another request reserved between our Lookup and Reserve.
				// Look up again to determine the current state.
				cached, err := store.Lookup(ctx, idempotencyKey, userID)
				if err != nil {
					logger.WarnContext(ctx, "idempotency lookup failed, proceeding without cache",
						"error", err,
						"procedure", procedure,
					)
					return next(ctx, req)
				}

				if cached != nil && cached.ResponseBytes != nil {
					return handleReplay(ctx, logger, cached, reqHash, procedure, proc)
				}

				return nil, connect.NewError(connect.CodeAborted,
					fmt.Errorf("a request with this idempotency key is already being processed"))
			}

			logger.DebugContext(ctx, "idempotency key reserved, executing handler",
				"procedure", procedure,
				"idempotency_key", idempotencyKey,
			)

			// Execute the handler.
			resp, err := next(ctx, req)
			if err != nil {
				// Handler failed — unreserve so the key can be retried.
				if unreserveErr := store.Unreserve(ctx, idempotencyKey, userID); unreserveErr != nil {
					logger.WarnContext(ctx, "failed to unreserve idempotency key",
						"error", unreserveErr,
						"procedure", procedure,
					)
				}
				return resp, err
			}

			// Complete the reservation with the response.
			if err := completeReservation(ctx, store, resp, idempotencyKey, userID, proc); err != nil {
				logger.WarnContext(ctx, "failed to complete idempotency reservation",
					"error", err,
					"procedure", procedure,
				)
				// Unreserve so the key can be retried.
				if unreserveErr := store.Unreserve(ctx, idempotencyKey, userID); unreserveErr != nil {
					logger.WarnContext(ctx, "failed to unreserve idempotency key after completion failure",
						"error", unreserveErr,
						"procedure", procedure,
					)
				}
			} else {
				logger.DebugContext(ctx, "idempotency reservation completed",
					"procedure", procedure,
					"idempotency_key", idempotencyKey,
				)
				resp.Header().Set(HeaderIdempotencyStatus, "processing")
			}

			return resp, nil
		}
	}
}

func handleReplay(
	ctx context.Context,
	logger *slog.Logger,
	cached *CachedResponse,
	reqHash []byte,
	procedure string,
	proc Procedure,
) (connect.AnyResponse, error) {
	// Validate procedure matches.
	if cached.Procedure != procedure {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("idempotency key was used with a different procedure"))
	}

	// Validate request hash matches.
	if !hashEqual(cached.RequestHash, reqHash) {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("idempotency key was used with different request parameters"))
	}

	resp, err := proc.DeserializeResponse(cached.ResponseBytes)
	if err != nil {
		logger.ErrorContext(ctx, "failed to deserialize cached response",
			"error", err,
			"procedure", procedure,
		)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("internal error"))
	}

	// Resolve the current status via the procedure-specific resolver.
	status := "processing"
	resolved, err := proc.ResolveStatus(ctx, cached.ResourceID)
	if err != nil {
		logger.WarnContext(ctx, "failed to resolve status, defaulting to processing",
			"error", err,
			"procedure", procedure,
			"resource_id", cached.ResourceID,
		)
	} else {
		status = resolved
	}

	logger.DebugContext(ctx, "idempotency replay completed",
		"procedure", procedure,
		"resource_id", cached.ResourceID,
		"status", status,
	)

	resp.Header().Set(HeaderIdempotencyStatus, status)
	return resp, nil
}

func completeReservation(
	ctx context.Context,
	store *Store,
	resp connect.AnyResponse,
	idempotencyKey uuid.UUID,
	userID uuid.UUID,
	proc Procedure,
) error {
	msg, ok := resp.Any().(proto.Message)
	if !ok {
		return fmt.Errorf("response is not a proto.Message")
	}

	responseBytes, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}

	resourceID, err := proc.ExtractResourceID(resp.Any())
	if err != nil {
		return fmt.Errorf("extract resource ID: %w", err)
	}

	err = store.Complete(ctx, &CompleteParams{
		IdempotencyKey: idempotencyKey,
		UserID:         userID,
		ResponseBytes:  responseBytes,
		ResourceType:   proc.ResourceType(),
		ResourceID:     resourceID,
	})
	if err != nil {
		return fmt.Errorf("complete: %w", err)
	}

	return nil
}

func hashRequest(req connect.AnyRequest) ([]byte, error) {
	msg, ok := req.Any().(proto.Message)
	if !ok {
		return nil, fmt.Errorf("request is not a proto.Message")
	}

	// Use deterministic marshaling for consistent hashes.
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	h := sha256.Sum256(b)
	return h[:], nil
}

func hashEqual(a, b []byte) bool {
	if a == nil || b == nil {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}
