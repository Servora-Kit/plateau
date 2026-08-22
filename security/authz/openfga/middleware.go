package openfga

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	authzpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/authz/v1"
	securityerrorspb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/errors/v1"
	security "github.com/Servora-Kit/plateau/security"
	authzruntime "github.com/Servora-Kit/plateau/security/authz"
	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
	fgasdk "github.com/openfga/go-sdk"
	fgaclient "github.com/openfga/go-sdk/client"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Server constructs OpenFGA-specific route authorization middleware.
func Server(authorizer *Authorizer, opts ...authzruntime.Option) middleware.Middleware {
	if !validAuthorizer(authorizer) {
		panic("openfga authz: authorizer is invalid")
	}
	rules := authzruntime.NewRules(opts...)
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, request any) (any, error) {
			if ctx == nil {
				return nil, apiError(fmt.Errorf("openfga authz: context is nil"))
			}
			serverTransport, ok := transport.FromServerContext(ctx)
			if !ok || serverTransport == nil {
				return nil, apiError(fmt.Errorf("openfga authz: server transport is missing"))
			}
			operation := serverTransport.Operation()
			rule := rules[operation]
			if rule == nil {
				return nil, apiError(fmt.Errorf("openfga authz: no authorization rule for operation %q", operation))
			}
			mode := rule.GetMode()
			switch mode {
			case authzpb.AuthzMode_AUTHZ_MODE_NONE:
				return handler(ctx, request)
			case authzpb.AuthzMode_AUTHZ_MODE_REQUIRED:
			default:
				return nil, apiError(fmt.Errorf("openfga authz: unsupported mode %s", mode))
			}

			actor, ok := security.ActorFrom(ctx)
			if !ok || actor.Type == security.ActorTypeAnonymous {
				return nil, apiError(fmt.Errorf("%w: authenticated actor is missing", ErrUnauthenticated))
			}
			resourceID := rule.GetResourceId()
			if resourceID == "" {
				fieldPath := rule.GetResourceIdField()
				if fieldPath == "" {
					return nil, apiError(fmt.Errorf("openfga authz: authorization rule target is missing"))
				}
				resolved, err := resourceIDFromRequest(fieldPath, request)
				if err != nil {
					return nil, apiError(fmt.Errorf("%w: %w", ErrInvalidInput, err))
				}
				resourceID = resolved
			}
			allowed, err := authorizer.Check(ctx, actor, rule.GetAction(), rule.GetResourceType(), resourceID)
			if err != nil {
				return nil, apiError(err)
			}
			if !allowed {
				return nil, securityerrorspb.ErrorSecurityErrorReasonPermissionDenied("permission denied")
			}
			return handler(ctx, request)
		}
	}
}

func resourceIDFromRequest(fieldPath string, request any) (string, error) {
	message, ok := request.(proto.Message)
	if !ok {
		return "", fmt.Errorf("openfga authz: request is not a proto message")
	}
	current := message.ProtoReflect()
	if !current.IsValid() {
		return "", fmt.Errorf("openfga authz: request proto message is invalid")
	}
	segments := strings.Split(fieldPath, ".")
	for index, segment := range segments {
		field := current.Descriptor().Fields().ByName(protoreflect.Name(segment))
		if field == nil {
			return "", fmt.Errorf("openfga authz: resource field %q is missing", strings.Join(segments[:index+1], "."))
		}
		if oneof := field.ContainingOneof(); oneof != nil && !oneof.IsSynthetic() {
			return "", fmt.Errorf("openfga authz: resource field %q belongs to a oneof", strings.Join(segments[:index+1], "."))
		}
		if index < len(segments)-1 {
			if (field.Kind() != protoreflect.MessageKind && field.Kind() != protoreflect.GroupKind) || field.IsList() || field.IsMap() || !current.Has(field) {
				return "", fmt.Errorf("openfga authz: resource field %q is unavailable", strings.Join(segments[:index+1], "."))
			}
			current = current.Get(field).Message()
			continue
		}
		if field.IsList() || field.IsMap() {
			return "", fmt.Errorf("openfga authz: resource field %q is not singular", fieldPath)
		}
		if field.HasPresence() && !current.Has(field) {
			return "", fmt.Errorf("openfga authz: resource field %q is unavailable", fieldPath)
		}
		value := current.Get(field)
		var resourceID string
		switch field.Kind() {
		case protoreflect.StringKind:
			resourceID = value.String()
		case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
			protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
			resourceID = strconv.FormatInt(value.Int(), 10)
		case protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
			protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
			resourceID = strconv.FormatUint(value.Uint(), 10)
		default:
			return "", fmt.Errorf("openfga authz: resource field %q has unsupported kind %s", fieldPath, field.Kind())
		}
		if strings.TrimSpace(resourceID) == "" {
			return "", fmt.Errorf("openfga authz: resource field %q is empty", fieldPath)
		}
		return resourceID, nil
	}
	return "", fmt.Errorf("openfga authz: resource field path is empty")
}

func apiError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	switch {
	case errors.Is(err, ErrUnauthenticated):
		return securityerrorspb.ErrorSecurityErrorReasonUnauthenticated("authentication required").WithCause(err)
	case errors.Is(err, ErrInvalidInput):
		return securityerrorspb.ErrorSecurityErrorReasonInvalidArgument("invalid authorization argument").WithCause(err)
	case errors.Is(err, ErrUnavailable):
		return securityerrorspb.ErrorSecurityErrorReasonUnavailable("authorization service unavailable").WithCause(err)
	}

	var rateLimit fgasdk.FgaApiRateLimitExceededError
	var providerInternal fgasdk.FgaApiInternalError
	if errors.As(err, &rateLimit) || errors.As(err, &providerInternal) {
		return securityerrorspb.ErrorSecurityErrorReasonUnavailable("authorization service unavailable").WithCause(err)
	}

	var required fgaclient.FgaRequiredParamError
	var invalid fgaclient.FgaInvalidError
	var validation fgasdk.FgaApiValidationError
	var unsupportedType *json.UnsupportedTypeError
	var unsupportedValue *json.UnsupportedValueError
	if errors.As(err, &required) || errors.As(err, &invalid) || errors.As(err, &validation) ||
		errors.As(err, &unsupportedType) || errors.As(err, &unsupportedValue) {
		return securityerrorspb.ErrorSecurityErrorReasonInvalidArgument("invalid authorization argument").WithCause(err)
	}

	var networkError net.Error
	if errors.As(err, &networkError) {
		return securityerrorspb.ErrorSecurityErrorReasonUnavailable("authorization service unavailable").WithCause(err)
	}
	return securityerrorspb.ErrorSecurityErrorReasonInternal("internal authorization error").WithCause(err)
}
