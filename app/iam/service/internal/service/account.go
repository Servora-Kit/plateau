package service

import (
	"context"
	"errors"
	"fmt"

	accountpb "github.com/Servora-Kit/plateau/api/gen/go/iam/account/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/authn"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	kerrors "github.com/go-kratos/kratos/v3/errors"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// AccountService exposes public registration, profile and recovery flows.
type AccountService struct {
	accountpb.UnimplementedAccountServiceServer
	account *biz.AccountUsecase
}

func NewAccountService(account *biz.AccountUsecase) (*AccountService, error) {
	if account == nil {
		return nil, fmt.Errorf("account service: usecase is nil")
	}
	return &AccountService{account: account}, nil
}

func (s *AccountService) Register(ctx context.Context, request *accountpb.RegisterRequest) (*accountpb.RegisterResponse, error) {
	if request == nil {
		return nil, accountError(biz.ErrInvalidToken)
	}
	user, err := s.account.Register(ctx, request.GetEmail(), request.GetPassword(), request.GetCapToken(), request.GetProfile())
	if err != nil {
		return nil, accountError(err)
	}
	return &accountpb.RegisterResponse{User: user}, nil
}

func (s *AccountService) VerifyEmail(ctx context.Context, request *accountpb.VerifyEmailRequest) (*accountpb.VerifyEmailResponse, error) {
	if request == nil {
		return nil, accountError(biz.ErrInvalidToken)
	}
	user, err := s.account.VerifyEmail(ctx, request.GetToken())
	if err != nil {
		return nil, accountError(err)
	}
	return &accountpb.VerifyEmailResponse{User: user}, nil
}

func (s *AccountService) ResendVerificationEmail(ctx context.Context, request *accountpb.ResendVerificationEmailRequest) (*accountpb.ResendVerificationEmailResponse, error) {
	if request == nil {
		return nil, accountError(biz.ErrInvalidToken)
	}
	if err := s.account.ResendVerification(ctx, request.GetEmail(), request.GetCapToken()); err != nil {
		return nil, accountError(err)
	}
	return &accountpb.ResendVerificationEmailResponse{}, nil
}

func (s *AccountService) RequestPasswordReset(ctx context.Context, request *accountpb.RequestPasswordResetRequest) (*accountpb.RequestPasswordResetResponse, error) {
	if request == nil {
		return nil, accountError(biz.ErrInvalidToken)
	}
	if err := s.account.RequestPasswordReset(ctx, request.GetEmail(), request.GetCapToken()); err != nil {
		return nil, accountError(err)
	}
	return &accountpb.RequestPasswordResetResponse{}, nil
}

func (s *AccountService) ConfirmPasswordReset(ctx context.Context, request *accountpb.ConfirmPasswordResetRequest) (*accountpb.ConfirmPasswordResetResponse, error) {
	if request == nil {
		return nil, accountError(biz.ErrInvalidToken)
	}
	if err := s.account.ConfirmPasswordReset(ctx, request.GetToken(), request.GetNewPassword()); err != nil {
		return nil, accountError(err)
	}
	return &accountpb.ConfirmPasswordResetResponse{}, nil
}

func (s *AccountService) GetProfile(ctx context.Context, _ *accountpb.GetProfileRequest) (*accountpb.GetProfileResponse, error) {
	user, _, err := authn.From(ctx)
	if err != nil {
		return nil, accountError(err)
	}
	return &accountpb.GetProfileResponse{User: user}, nil
}

func (s *AccountService) UpdateProfile(ctx context.Context, request *accountpb.UpdateProfileRequest) (*accountpb.UpdateProfileResponse, error) {
	if request == nil || request.GetProfile() == nil {
		return nil, kerrors.New(400, "INVALID_ARGUMENT", "profile is required")
	}
	user, _, err := authn.From(ctx)
	if err != nil {
		return nil, accountError(err)
	}
	profile, err := mergeProfile(user.GetProfile(), request.GetProfile(), request.GetUpdateMask())
	if err != nil {
		return nil, err
	}
	updated, err := s.account.UpdateProfile(ctx, user.GetUserId(), user.GetEtag(), profile)
	if err != nil {
		return nil, accountError(err)
	}
	return &accountpb.UpdateProfileResponse{User: updated}, nil
}

func (s *AccountService) ChangePassword(ctx context.Context, request *accountpb.ChangePasswordRequest) (*accountpb.ChangePasswordResponse, error) {
	if request == nil {
		return nil, accountError(biz.ErrInvalidCredentials)
	}
	user, loginSession, err := authn.From(ctx)
	if err != nil {
		return nil, accountError(err)
	}
	if err := s.account.ChangePassword(ctx, user.GetUserId(), loginSession.GetSessionId(), request.GetCurrentPassword(), request.GetNewPassword()); err != nil {
		return nil, accountError(err)
	}
	return &accountpb.ChangePasswordResponse{}, nil
}

func mergeProfile(current, requested *userpb.UserProfile, mask *fieldmaskpb.FieldMask) (*userpb.UserProfile, error) {
	if requested == nil {
		return nil, fmt.Errorf("profile is required")
	}
	if mask == nil || len(mask.Paths) == 0 {
		return requested, nil
	}
	merged := current
	if merged == nil {
		merged = new(userpb.UserProfile)
	}
	for _, path := range mask.Paths {
		switch path {
		case "name":
			merged.Name = requested.Name
		case "given_name":
			merged.GivenName = requested.GivenName
		case "family_name":
			merged.FamilyName = requested.FamilyName
		case "nickname":
			merged.Nickname = requested.Nickname
		case "preferred_username":
			merged.PreferredUsername = requested.PreferredUsername
		case "picture":
			merged.Picture = requested.Picture
		case "locale":
			merged.Locale = requested.Locale
		default:
			return nil, fmt.Errorf("unsupported profile field %q", path)
		}
	}
	return merged, nil
}

func accountError(err error) error {
	switch {
	case errors.Is(err, biz.ErrInvalidToken):
		return accountpb.ErrorAccountErrorReasonInvalidToken("token is invalid")
	case errors.Is(err, biz.ErrTokenExpired):
		return accountpb.ErrorAccountErrorReasonTokenExpired("token is expired")
	case errors.Is(err, biz.ErrInvalidCredentials):
		return accountpb.ErrorAccountErrorReasonInvalidPassword("password verification failed")
	case errors.Is(err, biz.ErrInvalidPassword):
		return accountpb.ErrorAccountErrorReasonInvalidPassword("password does not meet policy")
	case errors.Is(err, biz.ErrEmailAlreadyExists):
		return accountpb.ErrorAccountErrorReasonEmailAlreadyRegistered("email is already registered")
	case errors.Is(err, biz.ErrProfileEtagMismatch):
		return kerrors.New(409, "ETAG_MISMATCH", "profile was modified by another request").WithCause(err)
	case errors.Is(err, biz.ErrUnauthenticated), errors.Is(err, biz.ErrSessionRevoked), errors.Is(err, biz.ErrUserDisabled), errors.Is(err, biz.ErrUserNotActive):
		return accountpb.ErrorAccountErrorReasonUnauthenticated("authentication required")
	default:
		return kerrors.InternalServer("IAM account operation failed", "account operation failed").WithCause(err)
	}
}
