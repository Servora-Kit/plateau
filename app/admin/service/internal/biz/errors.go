package biz

import "errors"

var (
	// ErrUnauthenticated indicates that no valid human Actor is present in context.
	ErrUnauthenticated = errors.New("admin: actor is unauthenticated")

	// ErrSelfActionForbidden rejects self-disable, self-delete, and self-expunge attempts.
	ErrSelfActionForbidden = errors.New("admin: cannot disable or delete current administrator")

	// ErrTargetIsAdmin rejects disabling or deleting another user who holds Admin privileges.
	ErrTargetIsAdmin = errors.New("admin: target user holds administrator privileges")

	// ErrInvalidUpdateMask rejects empty or disallowed field paths in UpdateUser.
	ErrInvalidUpdateMask = errors.New("admin: update_mask contains unsupported or immutable fields")

	// ErrUserNotFound indicates that the target user does not exist or is not in the expected deletion state.
	ErrUserNotFound = errors.New("admin: user not found")

	// ErrUserAlreadyExists indicates that the email is already occupied or the user is already active.
	ErrUserAlreadyExists = errors.New("admin: user already exists")

	// ErrUserInvalidState indicates that the user's current lifecycle state forbids the operation.
	ErrUserInvalidState = errors.New("admin: user lifecycle state is invalid")

	// ErrUserEtagMismatch indicates an optimistic concurrency conflict on the user resource.
	ErrUserEtagMismatch = errors.New("admin: user etag mismatch")

	// ErrVerificationEmailFailed indicates that verification email delivery failed downstream.
	ErrVerificationEmailFailed = errors.New("admin: verification email delivery failed")

	// ErrDownstreamUnavailable indicates a downstream IAM/OpenFGA or service-credential dependency failure.
	ErrDownstreamUnavailable = errors.New("admin: downstream service unavailable")
)
