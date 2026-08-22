// Package openfga provides concrete OpenFGA authorization semantics.
package openfga

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	security "github.com/Servora-Kit/plateau/security"
	fgasdk "github.com/openfga/go-sdk"
	fgaclient "github.com/openfga/go-sdk/client"
)

// SubjectMapper maps one authenticated Actor to a concrete OpenFGA direct subject.
type SubjectMapper func(security.Actor) (string, error)

// Authorizer maps stable Actors and explicit resources to the official SDK.
type Authorizer struct {
	client     *fgaclient.OpenFgaClient
	mapSubject SubjectMapper
}

// Request is one concrete OpenFGA authorization check.
type Request struct {
	Actor        security.Actor
	Action       string
	ResourceType string
	ResourceID   string
}

// New constructs an OpenFGA authorization capability with service-owned subject mapping.
func New(client *fgaclient.OpenFgaClient, mapSubject SubjectMapper) (*Authorizer, error) {
	if !validSDKClient(client) {
		return nil, fmt.Errorf("openfga authz: sdk client is invalid")
	}
	if mapSubject == nil {
		return nil, fmt.Errorf("openfga authz: subject mapper is nil")
	}
	return &Authorizer{client: client, mapSubject: mapSubject}, nil
}

// Check performs one OpenFGA authorization check.
func (authorizer *Authorizer) Check(ctx context.Context, actor security.Actor, action, resourceType, resourceID string) (bool, error) {
	if ctx == nil {
		return false, fmt.Errorf("%w: context is nil", ErrInvalidInput)
	}
	if !validAuthorizer(authorizer) {
		return false, fmt.Errorf("openfga authz: authorizer is invalid")
	}
	user, relation, object, err := authorizer.requestParts(actor, action, resourceType, resourceID)
	if err != nil {
		return false, err
	}
	response, err := authorizer.client.Check(ctx).Body(fgaclient.ClientCheckRequest{
		User: user, Relation: relation, Object: object,
	}).Execute()
	if err != nil {
		return false, fmt.Errorf("openfga check: %w", err)
	}
	if response == nil || !response.HasAllowed() {
		return false, fmt.Errorf("openfga check: response is missing allowed")
	}
	return response.GetAllowed(), nil
}

// BatchCheck performs ordered OpenFGA checks and fails as one operation.
func (authorizer *Authorizer) BatchCheck(ctx context.Context, requests []Request) ([]bool, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: context is nil", ErrInvalidInput)
	}
	if !validAuthorizer(authorizer) {
		return nil, fmt.Errorf("openfga authz: authorizer is invalid")
	}
	if len(requests) == 0 {
		return []bool{}, nil
	}
	items := make([]fgaclient.ClientBatchCheckItem, len(requests))
	for index, request := range requests {
		user, relation, object, err := authorizer.requestParts(request.Actor, request.Action, request.ResourceType, request.ResourceID)
		if err != nil {
			return nil, fmt.Errorf("openfga batch check item[%d]: %w", index, err)
		}
		items[index] = fgaclient.ClientBatchCheckItem{
			User: user, Relation: relation, Object: object, CorrelationId: strconv.Itoa(index),
		}
	}

	response, err := authorizer.client.BatchCheck(ctx).Body(fgaclient.ClientBatchCheckRequest{Checks: items}).Execute()
	if err != nil {
		return nil, fmt.Errorf("openfga batch check: %w", err)
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
				return nil, fmt.Errorf("%w: batch check item[%d] internal error: %s", ErrUnavailable, index, code)
			}
			if code := checkErr.GetInputError(); code != "" && code != fgasdk.ERRORCODE_NO_ERROR {
				return nil, fmt.Errorf("%w: batch check item[%d] input error: %s", ErrInvalidInput, index, code)
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

// ListAllowed returns bare resource IDs allowed for one Actor and action.
func (authorizer *Authorizer) ListAllowed(ctx context.Context, actor security.Actor, action, resourceType string) ([]string, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: context is nil", ErrInvalidInput)
	}
	if !validAuthorizer(authorizer) {
		return nil, fmt.Errorf("openfga authz: authorizer is invalid")
	}
	user, err := authorizer.subject(actor)
	if err != nil {
		return nil, err
	}
	relation, err := relationName(action)
	if err != nil {
		return nil, err
	}
	resourceType, err = resourceTypeName(resourceType)
	if err != nil {
		return nil, err
	}
	response, err := authorizer.client.ListObjects(ctx).Body(fgaclient.ClientListObjectsRequest{
		User: user, Relation: relation, Type: resourceType,
	}).Execute()
	if err != nil {
		return nil, fmt.Errorf("openfga list objects: %w", err)
	}
	if response == nil {
		return nil, fmt.Errorf("openfga list objects: response is nil")
	}
	objects := response.GetObjects()
	prefix := resourceType + ":"
	ids := make([]string, len(objects))
	for index, providerObject := range objects {
		if !strings.HasPrefix(providerObject, prefix) {
			return nil, fmt.Errorf("openfga list objects: object[%d] has unexpected resource type", index)
		}
		id := strings.TrimPrefix(providerObject, prefix)
		if !validResourceID(id, 256-utf8.RuneCountInString(prefix)) {
			return nil, fmt.Errorf("openfga list objects: object[%d] has invalid resource ID", index)
		}
		ids[index] = id
	}
	return ids, nil
}

func (authorizer *Authorizer) requestParts(actor security.Actor, action, resourceType, resourceID string) (string, string, string, error) {
	user, err := authorizer.subject(actor)
	if err != nil {
		return "", "", "", err
	}
	relation, err := relationName(action)
	if err != nil {
		return "", "", "", err
	}
	object, err := resourceObject(resourceType, resourceID)
	if err != nil {
		return "", "", "", err
	}
	return user, relation, object, nil
}

func (authorizer *Authorizer) subject(actor security.Actor) (string, error) {
	if !actor.Valid() {
		return "", fmt.Errorf("%w: actor is invalid", ErrInvalidInput)
	}
	if actor.Type == security.ActorTypeAnonymous {
		return "", fmt.Errorf("%w: anonymous actor cannot access protected resources", ErrUnauthenticated)
	}
	subject, err := authorizer.mapSubject(actor)
	if err != nil {
		return "", fmt.Errorf("openfga authz: map Actor subject: %w", err)
	}
	subjectType, subjectID, ok := strings.Cut(subject, ":")
	if !ok || !validProviderName(subjectType, 254) || subjectID == "*" || !validResourceID(subjectID, 512-utf8.RuneCountInString(subjectType)-1) {
		return "", fmt.Errorf("%w: mapped provider subject is invalid", ErrInvalidInput)
	}
	return subject, nil
}

func relationName(action string) (string, error) {
	if !validProviderName(action, 50) {
		return "", fmt.Errorf("%w: action is invalid", ErrInvalidInput)
	}
	return action, nil
}

func resourceTypeName(resourceType string) (string, error) {
	if !validProviderName(resourceType, 254) {
		return "", fmt.Errorf("%w: resource type is invalid", ErrInvalidInput)
	}
	return resourceType, nil
}

func resourceObject(resourceType, resourceID string) (string, error) {
	resourceType, err := resourceTypeName(resourceType)
	if err != nil {
		return "", err
	}
	if !validResourceID(resourceID, 256-utf8.RuneCountInString(resourceType)-1) {
		return "", fmt.Errorf("%w: resource ID is invalid", ErrInvalidInput)
	}
	return resourceType + ":" + resourceID, nil
}

func validProviderName(value string, maxLength int) bool {
	if value == "" || len(value) > maxLength || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if !(character == '_' || character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' || character >= '0' && character <= '9') {
			return false
		}
	}
	return true
}

func validResourceID(value string, maxLength int) bool {
	if value == "" || maxLength < 1 || !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxLength {
		return false
	}
	return !strings.ContainsAny(value, "#:") &&
		strings.IndexFunc(value, func(character rune) bool {
			return unicode.IsSpace(character) || unicode.IsControl(character)
		}) == -1
}

func validSDKClient(client *fgaclient.OpenFgaClient) bool {
	return client != nil && client.APIClient.OpenFgaApi != nil
}

func validAuthorizer(authorizer *Authorizer) bool {
	return authorizer != nil && validSDKClient(authorizer.client) && authorizer.mapSubject != nil
}
