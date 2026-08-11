// Package openfga adapts the official OpenFGA SDK client to Platform
// authorization capabilities.
package openfga

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/Servora-Kit/servora-platform/security/authz"
	fgasdk "github.com/openfga/go-sdk"
	fgaclient "github.com/openfga/go-sdk/client"
)

// Option configures Authorizer construction.
type Option func(*adapterConfig) error

type adapterConfig struct {
	logger *slog.Logger
}

// WithLogger configures provider-local diagnostics.
func WithLogger(logger *slog.Logger) Option {
	return func(config *adapterConfig) error {
		if logger == nil {
			return fmt.Errorf("openfga authz: logger is nil")
		}
		config.logger = logger
		return nil
	}
}

// Authorizer maps Platform authorization requests to the official OpenFGA SDK.
type Authorizer struct {
	client *fgaclient.OpenFgaClient
	logger *slog.Logger
}

var (
	_ authz.Authorizer      = (*Authorizer)(nil)
	_ authz.BatchAuthorizer = (*Authorizer)(nil)
	_ authz.Lister          = (*Authorizer)(nil)
)

// New constructs an OpenFGA authorization adapter.
func New(client *fgaclient.OpenFgaClient, options ...Option) (*Authorizer, error) {
	if client == nil {
		return nil, fmt.Errorf("openfga authz: SDK client is nil")
	}
	var config adapterConfig
	for index, option := range options {
		if option == nil {
			return nil, fmt.Errorf("openfga authz: option[%d] is nil", index)
		}
		if err := option(&config); err != nil {
			return nil, fmt.Errorf("openfga authz: option[%d]: %w", index, err)
		}
	}
	return &Authorizer{client: client, logger: config.logger}, nil
}

// Check implements authz.Authorizer.
func (authorizer *Authorizer) Check(ctx context.Context, request authz.CheckRequest) (bool, error) {
	if err := validateCheckRequest(request); err != nil {
		return false, err
	}
	body := fgaclient.ClientCheckRequest{
		User:     request.Subject,
		Relation: request.Action,
		Object:   request.Resource.Type + ":" + request.Resource.ID,
	}
	response, err := authorizer.client.Check(ctx).Body(body).Execute()
	if err != nil {
		classified := classifyProviderError(ctx, "check", err)
		authorizer.logProviderError(ctx, "check", classified)
		return false, classified
	}
	if response == nil || !response.HasAllowed() {
		return false, fmt.Errorf("openfga check: response is missing allowed")
	}
	return response.GetAllowed(), nil
}

// BatchCheck implements authz.BatchAuthorizer and preserves input order.
func (authorizer *Authorizer) BatchCheck(ctx context.Context, requests []authz.CheckRequest) ([]bool, error) {
	if len(requests) == 0 {
		return []bool{}, nil
	}
	items := make([]fgaclient.ClientBatchCheckItem, len(requests))
	for index, request := range requests {
		if err := validateCheckRequest(request); err != nil {
			return nil, fmt.Errorf("openfga batch check item[%d]: %w", index, err)
		}
		item := fgaclient.ClientBatchCheckItem{
			User:          request.Subject,
			Relation:      request.Action,
			Object:        request.Resource.Type + ":" + request.Resource.ID,
			CorrelationId: strconv.Itoa(index),
		}
		items[index] = item
	}

	response, err := authorizer.client.BatchCheck(ctx).Body(fgaclient.ClientBatchCheckRequest{Checks: items}).Execute()
	if err != nil {
		classified := classifyProviderError(ctx, "batch check", err)
		authorizer.logProviderError(ctx, "batch_check", classified)
		return nil, classified
	}
	if response == nil {
		return nil, fmt.Errorf("openfga batch check: response is nil")
	}
	results := response.GetResult()
	if len(results) != len(requests) {
		return nil, fmt.Errorf("openfga batch check: result cardinality %d does not match request cardinality %d", len(results), len(requests))
	}

	allowed := make([]bool, len(requests))
	for index := range requests {
		correlationID := strconv.Itoa(index)
		result, ok := results[correlationID]
		if !ok {
			return nil, fmt.Errorf("openfga batch check: missing correlation ID %q", correlationID)
		}
		if checkErr, hasError := result.GetErrorOk(); hasError {
			if code := checkErr.GetInternalError(); code != "" && code != fgasdk.INTERNALERRORCODE_NO_INTERNAL_ERROR {
				cause := fmt.Errorf("openfga batch check item[%d] internal error: %s", index, code)
				return nil, fmt.Errorf("%w: %w", authz.ErrUnavailable, cause)
			}
			if code := checkErr.GetInputError(); code != "" && code != fgasdk.ERRORCODE_NO_ERROR {
				return nil, fmt.Errorf("openfga batch check item[%d] input error: %s", index, code)
			}
			return nil, fmt.Errorf("openfga batch check item[%d] returned an unspecified error", index)
		}
		if !result.HasAllowed() {
			return nil, fmt.Errorf("openfga batch check item[%d] is missing allowed", index)
		}
		allowed[index] = result.GetAllowed()
	}
	return allowed, nil
}

// ListAllowed implements authz.Lister and returns bare resource IDs.
func (authorizer *Authorizer) ListAllowed(ctx context.Context, subject, action, resourceType string) ([]string, error) {
	if err := validateListRequest(subject, action, resourceType); err != nil {
		return nil, err
	}
	response, err := authorizer.client.ListObjects(ctx).Body(fgaclient.ClientListObjectsRequest{
		User:     subject,
		Relation: action,
		Type:     resourceType,
	}).Execute()
	if err != nil {
		classified := classifyProviderError(ctx, "list objects", err)
		authorizer.logProviderError(ctx, "list_objects", classified)
		return nil, classified
	}
	if response == nil {
		return nil, fmt.Errorf("openfga list objects: response is nil")
	}
	objects := response.GetObjects()
	prefix := resourceType + ":"
	ids := make([]string, len(objects))
	for index, object := range objects {
		if !strings.HasPrefix(object, prefix) {
			return nil, fmt.Errorf("openfga list objects: object[%d] %q does not match resource type %q", index, object, resourceType)
		}
		id := strings.TrimPrefix(object, prefix)
		if id == "" {
			return nil, fmt.Errorf("openfga list objects: object[%d] has empty resource ID", index)
		}
		ids[index] = id
	}
	return ids, nil
}

func validateCheckRequest(request authz.CheckRequest) error {
	if strings.TrimSpace(request.Subject) == "" {
		return fmt.Errorf("openfga authz: subject is empty")
	}
	if strings.TrimSpace(request.Action) == "" {
		return fmt.Errorf("openfga authz: action is empty")
	}
	if strings.TrimSpace(request.Resource.Type) == "" {
		return fmt.Errorf("openfga authz: resource type is empty")
	}
	if strings.TrimSpace(request.Resource.ID) == "" {
		return fmt.Errorf("openfga authz: resource ID is empty")
	}
	return nil
}

func validateListRequest(subject, action, resourceType string) error {
	if strings.TrimSpace(subject) == "" {
		return fmt.Errorf("openfga authz: subject is empty")
	}
	if strings.TrimSpace(action) == "" {
		return fmt.Errorf("openfga authz: action is empty")
	}
	if strings.TrimSpace(resourceType) == "" {
		return fmt.Errorf("openfga authz: resource type is empty")
	}
	return nil
}

func classifyProviderError(ctx context.Context, operation string, err error) error {
	if err == nil {
		return nil
	}
	if contextErr := ctx.Err(); contextErr != nil && !stderrors.Is(err, contextErr) {
		err = stderrors.Join(contextErr, err)
	}
	wrapped := fmt.Errorf("openfga %s: %w", operation, err)
	if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
		return wrapped
	}
	var rateLimit fgasdk.FgaApiRateLimitExceededError
	var internal fgasdk.FgaApiInternalError
	if stderrors.As(err, &rateLimit) || stderrors.As(err, &internal) {
		return fmt.Errorf("%w: %w", authz.ErrUnavailable, wrapped)
	}

	var required fgaclient.FgaRequiredParamError
	var invalid fgaclient.FgaInvalidError
	var validation fgasdk.FgaApiValidationError
	var authentication fgasdk.FgaApiAuthenticationError
	var notFound fgasdk.FgaApiNotFoundError
	var apiError fgasdk.FgaApiError
	var generic fgasdk.GenericOpenAPIError
	var unsupportedType *json.UnsupportedTypeError
	var unsupportedValue *json.UnsupportedValueError
	if stderrors.As(err, &required) || stderrors.As(err, &invalid) ||
		stderrors.As(err, &validation) || stderrors.As(err, &authentication) ||
		stderrors.As(err, &notFound) || stderrors.As(err, &apiError) ||
		stderrors.As(err, &generic) || stderrors.As(err, &unsupportedType) ||
		stderrors.As(err, &unsupportedValue) {
		return wrapped
	}

	return fmt.Errorf("%w: %w", authz.ErrUnavailable, wrapped)
}

func (authorizer *Authorizer) logProviderError(ctx context.Context, operation string, err error) {
	if authorizer.logger == nil {
		return
	}
	reason := "internal"
	switch {
	case stderrors.Is(err, context.Canceled):
		reason = "canceled"
	case stderrors.Is(err, context.DeadlineExceeded):
		reason = "deadline_exceeded"
	case stderrors.Is(err, authz.ErrUnavailable):
		reason = "unavailable"
	}
	authorizer.logger.ErrorContext(ctx, "OpenFGA authorization request failed",
		"operation", operation,
		"reason", reason,
	)
}
