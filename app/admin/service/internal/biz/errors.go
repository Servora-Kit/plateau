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
)
