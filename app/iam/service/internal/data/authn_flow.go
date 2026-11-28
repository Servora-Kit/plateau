package data

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"slices"
	"time"

	authnpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/authn/v1"
	userpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/user/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/biz"
	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/authflow"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/oidcauthorizationrequest"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/passwordauthenticator"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent/user"
	crudmapper "github.com/Servora-Kit/plateau/crud/mapper"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type authFlowRepository struct {
	ent    *entmodel.Client
	mapper *crudmapper.ResourceMapper[*userpb.User, entmodel.User]
}

func NewAuthFlowRepository(data *Data) (biz.AuthFlowRepo, error) {
	if data == nil || data.ent == nil {
		return nil, fmt.Errorf("auth flow repository: data is nil")
	}
	mapper, err := newUserResourceMapper()
	if err != nil {
		return nil, err
	}
	return &authFlowRepository{ent: data.ent, mapper: mapper}, nil
}

func (repo *authFlowRepository) FindPendingOIDCRequest(
	ctx context.Context,
	requestID string,
	now time.Time,
) (*authnpb.AuthFlowOidcContext, time.Time, error) {
	req, client, err := loadPendingOIDCRequestAndClient(ctx, repo.ent, requestID, false, now.UTC())
	if err != nil {
		return nil, time.Time{}, err
	}
	return toAuthFlowOIDCContext(client), req.ExpiresTime.UTC(), nil
}

func (repo *authFlowRepository) Create(
	ctx context.Context,
	seed *authnpb.AuthFlow,
	ownerHash, oidcRequestID string,
	now time.Time,
) (*authnpb.AuthFlow, error) {
	if seed.GetFlowId() == "" || ownerHash == "" || seed.GetExpiresTime() == nil {
		return nil, fmt.Errorf("create auth flow: id, owner hash, and expires time are required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	expiresTime := seed.GetExpiresTime().AsTime().UTC()

	var (
		saved   *entmodel.AuthFlow
		oidcCtx *authnpb.AuthFlowOidcContext
	)
	err := inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		if oidcRequestID != "" {
			req, client, err := loadPendingOIDCRequestAndClient(ctx, tx.Client(), oidcRequestID, false, now)
			if errors.Is(err, biz.ErrNotFound) {
				return biz.ErrOIDCRequestInvalid
			}
			if err != nil {
				return err
			}
			oidcCtx = toAuthFlowOIDCContext(client)
			if req.ExpiresTime.UTC().Before(expiresTime) {
				expiresTime = req.ExpiresTime.UTC()
			}
		}
		if !now.Before(expiresTime) {
			return biz.ErrOIDCRequestInvalid
		}
		builder := tx.AuthFlow.Create().
			SetID(seed.GetFlowId()).
			SetOwnerHash(ownerHash).
			SetStatus(authflow.StatusActive).
			SetState(authflow.StateAuthenticate).
			SetCompletedAmr([]string{}).
			SetRequiredActions([]string{}).
			SetExpiresTime(expiresTime).
			SetCreateTime(now).
			SetUpdateTime(now)
		if oidcRequestID != "" {
			builder.SetOidcRequestID(oidcRequestID)
		}
		entity, err := builder.Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		saved = entity
		return nil
	})
	if err != nil {
		return nil, err
	}
	snap, err := toAuthFlowSnapshot(saved)
	if err != nil {
		return nil, err
	}
	snap.Flow.Oidc = oidcCtx
	return snap.Flow, nil
}

func (repo *authFlowRepository) Get(ctx context.Context, flowID string) (*biz.AuthFlowSnapshot, error) {
	if flowID == "" {
		return nil, biz.ErrNotFound
	}
	entity, err := repo.ent.AuthFlow.Get(ctx, flowID)
	if err != nil {
		return nil, translateEntError(err)
	}
	return toAuthFlowSnapshot(entity)
}

func (repo *authFlowRepository) Invalidate(ctx context.Context, flowID string, now time.Time) error {
	if flowID == "" {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	_, err := repo.ent.AuthFlow.Update().
		Where(
			authflow.IDEQ(flowID),
			authflow.StatusEQ(authflow.StatusActive),
		).
		SetStatus(authflow.StatusInvalidated).
		SetUpdateTime(now.UTC()).
		Save(ctx)
	return translateEntError(err)
}

func (repo *authFlowRepository) AdvanceAfterPassword(
	ctx context.Context,
	flowID, ownerHash string,
	credential biz.PasswordCredential,
	userEtag string,
	amr []string,
	stepTTL time.Duration,
	now time.Time,
) (*authnpb.AuthFlow, *biz.LoginSession, error) {
	if flowID == "" || ownerHash == "" || credential.UserID == "" || userEtag == "" {
		return nil, nil, fmt.Errorf("advance auth flow: missing required parameters")
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()

	var (
		outFlow      *authnpb.AuthFlow
		loginSession *biz.LoginSession
	)
	err := inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		// Fixed lock order: lockUser BEFORE locking auth_flows to prevent deadlocks with revokeUserSessions.
		current, err := lockUser(ctx, tx, credential.UserID)
		if err != nil {
			return err
		}
		flowRow, err := lockAuthFlow(ctx, tx, flowID)
		if err != nil {
			if errors.Is(err, biz.ErrNotFound) {
				return biz.ErrAuthFlowNotFound
			}
			return err
		}
		if subtle.ConstantTimeCompare([]byte(ownerHash), []byte(flowRow.OwnerHash)) != 1 {
			return biz.ErrAuthFlowNotFound
		}
		if isTerminalEntFlowStatus(flowRow.Status) {
			return biz.ErrAuthFlowNotFound
		}
		if !now.Before(flowRow.ExpiresTime) {
			return biz.ErrAuthFlowExpired
		}
		if flowRow.Status == authflow.StatusInvalidated {
			return biz.ErrAuthFlowInvalidated
		}
		if flowRow.State != authflow.StateAuthenticate {
			if flowRow.State == authflow.StateRequiredAction && (flowRow.StepExpiresTime == nil || !now.Before(*flowRow.StepExpiresTime)) {
				return biz.ErrAuthFlowExpired
			}
			return biz.ErrAuthFlowInvalidated
		}

		var oidcCtx *authnpb.AuthFlowOidcContext
		oidcRequestID := derefString(flowRow.OidcRequestID)
		if oidcRequestID != "" {
			_, client, err := loadPendingOIDCRequestAndClient(ctx, tx.Client(), oidcRequestID, false, now)
			if errors.Is(err, biz.ErrNotFound) {
				return biz.ErrOIDCRequestInvalid
			}
			if err != nil {
				return err
			}
			oidcCtx = toAuthFlowOIDCContext(client)
		}

		if current.Etag != userEtag {
			return biz.ErrMutationMiss
		}
		if current.Status != biz.UserStatusActive {
			return biz.ErrUserNotActive
		}
		if _, err := ensureVerifiedEmail(ctx, tx, current.ID); err != nil {
			if errors.Is(err, biz.ErrNotFound) || errors.Is(err, biz.ErrMutationMiss) {
				return biz.ErrUserNotActive
			}
			return err
		}
		passEntity, err := verifyPasswordCredentialSnapshot(ctx, tx, credential)
		if err != nil {
			if errors.Is(err, biz.ErrMutationMiss) {
				return biz.ErrInvalidCredentials
			}
			return err
		}

		resources, err := projectUsersWithClient(ctx, tx.Client(), repo.mapper, []*entmodel.User{current})
		if err != nil {
			return err
		}
		projectedUser := resources[0]

		completedAMR := biz.NormalizeAMR(append(slices.Clone(flowRow.CompletedAmr), amr...))
		progress := biz.EvaluateFlowProgress(completedAMR, biz.DefaultAuthPolicy(), projectedUser.GetRequiredActions())

		switch progress.GetState() {
		case authnpb.AuthFlowState_AUTH_FLOW_STATE_REQUIRED_ACTION:
			stepExpires := now.Add(stepTTL)
			if flowRow.ExpiresTime.Before(stepExpires) {
				stepExpires = flowRow.ExpiresTime
			}
			changedAt := biz.NormalizeTimestamp(passEntity.ChangedTime)
			saved, err := tx.AuthFlow.UpdateOneID(flowRow.ID).
				SetState(authflow.StateRequiredAction).
				SetUserID(current.ID).
				SetUserEmail(projectedUser.GetEmail()).
				SetUserEtag(current.Etag).
				SetAuthenticatorID(credential.AuthenticatorID).
				SetPasswordAuthenticatorID(credential.PasswordAuthenticatorID).
				SetPasswordChangedTime(changedAt).
				SetCompletedAmr(completedAMR).
				SetRequiredActions(slices.Clone(current.RequiredActions)).
				SetStepExpiresTime(stepExpires).
				SetUpdateTime(now).
				Save(ctx)
			if err != nil {
				return translateEntError(err)
			}
			snap, err := toAuthFlowSnapshot(saved)
			if err != nil {
				return err
			}
			snap.Flow.Oidc = oidcCtx
			outFlow = snap.Flow
			return nil

		case authnpb.AuthFlowState_AUTH_FLOW_STATE_COMPLETED:
			login, err := insertLoginSession(ctx, tx, current.ID, completedAMR, now)
			if err != nil {
				return err
			}
			saved, err := tx.AuthFlow.UpdateOneID(flowRow.ID).
				SetStatus(authflow.StatusCompleted).
				SetState(authflow.StateCompleted).
				SetUserID(current.ID).
				SetUserEmail(projectedUser.GetEmail()).
				SetCompletedAmr(completedAMR).
				SetRequiredActions([]string{}).
				ClearStepExpiresTime().
				SetUpdateTime(now).
				Save(ctx)
			if err != nil {
				return translateEntError(err)
			}
			outFlow = &authnpb.AuthFlow{
				FlowId:       saved.ID,
				State:        authnpb.AuthFlowState_AUTH_FLOW_STATE_COMPLETED,
				CompletedAmr: completedAMR,
				Email:        projectedUser.GetEmail(),
				Oidc:         oidcCtx,
				ExpiresTime:  timestamppb.New(saved.ExpiresTime.UTC()),
				ContinueUrl:  biz.BuildContinueURL(oidcRequestID),
				User:         projectedUser,
			}
			loginSession = login
			return nil

		default:
			return fmt.Errorf("unexpected evaluated flow state %v after password verification", progress.GetState())
		}
	})
	if err != nil {
		return nil, nil, err
	}
	return outFlow, loginSession, nil
}

func (repo *authFlowRepository) CompleteUpdatePassword(
	ctx context.Context,
	flowID, ownerHash, newPasswordHash string,
	stepTTL time.Duration,
	now time.Time,
) (*authnpb.AuthFlow, *biz.LoginSession, error) {
	if flowID == "" || ownerHash == "" || newPasswordHash == "" {
		return nil, nil, fmt.Errorf("complete update password: missing required parameters")
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()

	initialFlow, err := repo.ent.AuthFlow.Get(ctx, flowID)
	if entmodel.IsNotFound(err) {
		return nil, nil, biz.ErrAuthFlowNotFound
	}
	if err != nil {
		return nil, nil, translateEntError(err)
	}
	if subtle.ConstantTimeCompare([]byte(ownerHash), []byte(initialFlow.OwnerHash)) != 1 {
		return nil, nil, biz.ErrAuthFlowNotFound
	}
	if isTerminalEntFlowStatus(initialFlow.Status) {
		return nil, nil, biz.ErrAuthFlowNotFound
	}
	if !now.Before(initialFlow.ExpiresTime) {
		return nil, nil, biz.ErrAuthFlowExpired
	}
	if initialFlow.Status == authflow.StatusInvalidated {
		return nil, nil, biz.ErrAuthFlowInvalidated
	}
	targetUserID := derefString(initialFlow.UserID)
	if initialFlow.State != authflow.StateRequiredAction || targetUserID == "" {
		return nil, nil, biz.ErrAuthFlowInvalidated
	}
	if initialFlow.StepExpiresTime == nil || !now.Before(*initialFlow.StepExpiresTime) {
		return nil, nil, biz.ErrAuthFlowExpired
	}

	etag, err := biz.NewUserID()
	if err != nil {
		return nil, nil, err
	}

	var (
		outFlow         *authnpb.AuthFlow
		loginSession    *biz.LoginSession
		invalidatedInTx bool
		notFoundInTx    bool
	)
	err = inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		// Fixed lock order: lockUser BEFORE locking auth_flows.
		current, err := lockUser(ctx, tx, targetUserID)
		if errors.Is(err, biz.ErrNotFound) {
			affected, updateErr := tx.AuthFlow.Update().
				Where(authflow.IDEQ(flowID), authflow.StatusEQ(authflow.StatusActive)).
				SetStatus(authflow.StatusInvalidated).
				SetUpdateTime(now).
				Save(ctx)
			if updateErr != nil {
				return translateEntError(updateErr)
			}
			if affected == 0 {
				exists, existErr := tx.AuthFlow.Query().Where(authflow.IDEQ(flowID)).Exist(ctx)
				if existErr != nil {
					return translateEntError(existErr)
				}
				if !exists {
					notFoundInTx = true
					return nil
				}
			}
			invalidatedInTx = true
			return nil
		}
		if err != nil {
			return err
		}

		flowRow, err := lockAuthFlow(ctx, tx, flowID)
		if err != nil {
			if errors.Is(err, biz.ErrNotFound) {
				return biz.ErrAuthFlowNotFound
			}
			return err
		}
		if subtle.ConstantTimeCompare([]byte(ownerHash), []byte(flowRow.OwnerHash)) != 1 {
			return biz.ErrAuthFlowNotFound
		}
		if isTerminalEntFlowStatus(flowRow.Status) {
			return biz.ErrAuthFlowNotFound
		}
		if !now.Before(flowRow.ExpiresTime) {
			return biz.ErrAuthFlowExpired
		}
		if flowRow.Status == authflow.StatusInvalidated {
			return biz.ErrAuthFlowInvalidated
		}
		if flowRow.State != authflow.StateRequiredAction || derefString(flowRow.UserID) != current.ID {
			return biz.ErrAuthFlowInvalidated
		}
		if flowRow.StepExpiresTime == nil || !now.Before(*flowRow.StepExpiresTime) {
			return biz.ErrAuthFlowExpired
		}

		authEntity, passEntity, match, snapErr := verifyLockedRequiredActionSnapshot(ctx, tx, current, flowRow)
		if snapErr != nil {
			return snapErr
		}
		if !match {
			if _, err := tx.AuthFlow.UpdateOneID(flowRow.ID).
				SetStatus(authflow.StatusInvalidated).
				SetUpdateTime(now).
				Save(ctx); err != nil {
				return translateEntError(err)
			}
			invalidatedInTx = true
			return nil
		}

		changedAt := biz.NormalizeTimestamp(now)
		updatedPwd, err := tx.PasswordAuthenticator.Update().
			Where(
				passwordauthenticator.IDEQ(passEntity.ID),
				passwordauthenticator.AuthenticatorIDEQ(authEntity.ID),
				passwordauthenticator.PasswordHashEQ(passEntity.PasswordHash),
			).
			SetPasswordHash(newPasswordHash).
			SetChangedTime(changedAt).
			Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if updatedPwd != 1 {
			return biz.ErrAuthFlowInvalidated
		}

		remainingStored := make([]string, 0, len(current.RequiredActions))
		for _, action := range current.RequiredActions {
			if action != storedRequiredActionUpdatePassword {
				remainingStored = append(remainingStored, action)
			}
		}

		affectedUser, err := tx.User.Update().
			Where(
				user.IDEQ(current.ID),
				user.DeleteTimeIsNil(),
				user.EtagEQ(current.Etag),
			).
			SetRequiredActions(remainingStored).
			SetEtag(etag).
			SetUpdateTime(now).
			Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		if affectedUser != 1 {
			return biz.ErrAuthFlowInvalidated
		}

		if err := revokeUserSessionsAndFlows(ctx, tx, current.ID, "", flowRow.ID, now); err != nil {
			return err
		}

		updatedUserEntity, err := tx.User.Get(ctx, current.ID)
		if err != nil {
			return translateEntError(err)
		}
		resources, err := projectUsersWithClient(ctx, tx.Client(), repo.mapper, []*entmodel.User{updatedUserEntity})
		if err != nil {
			return err
		}
		projectedUser := resources[0]

		var oidcCtx *authnpb.AuthFlowOidcContext
		oidcRequestID := derefString(flowRow.OidcRequestID)
		if oidcRequestID != "" {
			_, client, reqErr := loadPendingOIDCRequestAndClient(ctx, tx.Client(), oidcRequestID, false, now)
			if reqErr == nil {
				oidcCtx = toAuthFlowOIDCContext(client)
			} else if !errors.Is(reqErr, biz.ErrNotFound) {
				return reqErr
			} else {
				oidcRequestID = ""
			}
		}

		completedAMR := biz.NormalizeAMR(flowRow.CompletedAmr)
		progress := biz.EvaluateFlowProgress(completedAMR, biz.DefaultAuthPolicy(), projectedUser.GetRequiredActions())
		if progress.GetState() == authnpb.AuthFlowState_AUTH_FLOW_STATE_COMPLETED {
			login, err := insertLoginSession(ctx, tx, current.ID, completedAMR, now)
			if err != nil {
				return err
			}
			savedFlow, err := tx.AuthFlow.UpdateOneID(flowRow.ID).
				SetStatus(authflow.StatusCompleted).
				SetState(authflow.StateCompleted).
				SetUserEmail(projectedUser.GetEmail()).
				SetUserEtag(etag).
				SetPasswordChangedTime(changedAt).
				SetCompletedAmr(completedAMR).
				SetRequiredActions([]string{}).
				ClearStepExpiresTime().
				SetUpdateTime(now).
				Save(ctx)
			if err != nil {
				return translateEntError(err)
			}
			outFlow = &authnpb.AuthFlow{
				FlowId:       savedFlow.ID,
				State:        authnpb.AuthFlowState_AUTH_FLOW_STATE_COMPLETED,
				CompletedAmr: completedAMR,
				Email:        projectedUser.GetEmail(),
				Oidc:         oidcCtx,
				ExpiresTime:  timestamppb.New(savedFlow.ExpiresTime.UTC()),
				ContinueUrl:  biz.BuildContinueURL(oidcRequestID),
				User:         projectedUser,
			}
			loginSession = login
			return nil
		}

		stepExpires := now.Add(stepTTL)
		if flowRow.ExpiresTime.Before(stepExpires) {
			stepExpires = flowRow.ExpiresTime
		}
		savedFlow, err := tx.AuthFlow.UpdateOneID(flowRow.ID).
			SetState(authflow.StateRequiredAction).
			SetUserEmail(projectedUser.GetEmail()).
			SetUserEtag(etag).
			SetPasswordChangedTime(changedAt).
			SetCompletedAmr(completedAMR).
			SetRequiredActions(remainingStored).
			SetStepExpiresTime(stepExpires).
			SetUpdateTime(now).
			Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		snap, err := toAuthFlowSnapshot(savedFlow)
		if err != nil {
			return err
		}
		snap.Flow.Oidc = oidcCtx
		outFlow = snap.Flow
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if notFoundInTx {
		return nil, nil, biz.ErrAuthFlowNotFound
	}
	if invalidatedInTx {
		return nil, nil, biz.ErrAuthFlowInvalidated
	}
	return outFlow, loginSession, nil
}

func (repo *authFlowRepository) Reset(
	ctx context.Context,
	flowID, newFlowID, ownerHash string,
	now time.Time,
) (*authnpb.AuthFlow, string, error) {
	if flowID == "" || newFlowID == "" || ownerHash == "" {
		return nil, "", fmt.Errorf("reset auth flow: missing required parameters")
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()

	var (
		savedNew    *entmodel.AuthFlow
		oidcCtx     *authnpb.AuthFlowOidcContext
		actorUserID string
	)
	err := inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		flowRow, err := lockAuthFlow(ctx, tx, flowID)
		if err != nil {
			if errors.Is(err, biz.ErrNotFound) {
				return biz.ErrAuthFlowNotFound
			}
			return err
		}
		if subtle.ConstantTimeCompare([]byte(ownerHash), []byte(flowRow.OwnerHash)) != 1 {
			return biz.ErrAuthFlowNotFound
		}
		if isTerminalEntFlowStatus(flowRow.Status) {
			return biz.ErrAuthFlowNotFound
		}
		if !now.Before(flowRow.ExpiresTime) {
			return biz.ErrAuthFlowExpired
		}
		actorUserID = derefString(flowRow.UserID)

		oidcRequestID := derefString(flowRow.OidcRequestID)
		if oidcRequestID != "" {
			_, client, err := loadPendingOIDCRequestAndClient(ctx, tx.Client(), oidcRequestID, false, now)
			if errors.Is(err, biz.ErrNotFound) {
				return biz.ErrOIDCRequestInvalid
			}
			if err != nil {
				return err
			}
			oidcCtx = toAuthFlowOIDCContext(client)
		}

		if _, err := tx.AuthFlow.UpdateOneID(flowRow.ID).
			SetStatus(authflow.StatusReset).
			SetUpdateTime(now).
			Save(ctx); err != nil {
			return translateEntError(err)
		}

		builder := tx.AuthFlow.Create().
			SetID(newFlowID).
			SetOwnerHash(flowRow.OwnerHash).
			SetStatus(authflow.StatusActive).
			SetState(authflow.StateAuthenticate).
			SetCompletedAmr([]string{}).
			SetRequiredActions([]string{}).
			SetExpiresTime(flowRow.ExpiresTime).
			SetCreateTime(now).
			SetUpdateTime(now)
		if oidcRequestID != "" {
			builder.SetOidcRequestID(oidcRequestID)
		}
		created, err := builder.Save(ctx)
		if err != nil {
			return translateEntError(err)
		}
		savedNew = created
		return nil
	})
	if err != nil {
		return nil, actorUserID, err
	}
	snap, err := toAuthFlowSnapshot(savedNew)
	if err != nil {
		return nil, actorUserID, err
	}
	snap.Flow.Oidc = oidcCtx
	return snap.Flow, actorUserID, nil
}

func (repo *authFlowRepository) Cancel(
	ctx context.Context,
	flowID, ownerHash string,
	now time.Time,
) (string, string, string, error) {
	if flowID == "" || ownerHash == "" {
		return "", "", "", fmt.Errorf("cancel auth flow: missing required parameters")
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()

	var (
		redirectURI string
		state       string
		actorUserID string
	)
	err := inTx(ctx, repo.ent, func(tx *entmodel.Tx) error {
		flowRow, err := lockAuthFlow(ctx, tx, flowID)
		if err != nil {
			if errors.Is(err, biz.ErrNotFound) {
				return biz.ErrAuthFlowNotFound
			}
			return err
		}
		if subtle.ConstantTimeCompare([]byte(ownerHash), []byte(flowRow.OwnerHash)) != 1 {
			return biz.ErrAuthFlowNotFound
		}
		if isTerminalEntFlowStatus(flowRow.Status) {
			return biz.ErrAuthFlowNotFound
		}
		if !now.Before(flowRow.ExpiresTime) {
			return biz.ErrAuthFlowExpired
		}
		if flowRow.Status == authflow.StatusInvalidated {
			return biz.ErrAuthFlowInvalidated
		}
		actorUserID = derefString(flowRow.UserID)

		if _, err := tx.AuthFlow.UpdateOneID(flowRow.ID).
			SetStatus(authflow.StatusCancelled).
			SetUpdateTime(now).
			Save(ctx); err != nil {
			return translateEntError(err)
		}

		oidcRequestID := derefString(flowRow.OidcRequestID)
		if oidcRequestID != "" {
			req, _, err := loadPendingOIDCRequestAndClient(ctx, tx.Client(), oidcRequestID, false, now)
			if err != nil && !errors.Is(err, biz.ErrNotFound) {
				return err
			}
			if req != nil {
				if err := tx.OIDCAuthorizationRequest.DeleteOneID(req.ID).Exec(ctx); err != nil {
					return translateEntError(err)
				}
				redirectURI = req.RedirectURI
				state = derefString(req.State)
			}
		}
		return nil
	})
	if err != nil {
		return "", "", actorUserID, err
	}
	return redirectURI, state, actorUserID, nil
}

func lockAuthFlow(ctx context.Context, tx *entmodel.Tx, flowID string) (*entmodel.AuthFlow, error) {
	entity, err := tx.AuthFlow.Query().Where(authflow.IDEQ(flowID)).ForUpdate().Only(ctx)
	return entity, translateEntError(err)
}

func loadPendingOIDCRequestAndClient(
	ctx context.Context,
	client *entmodel.Client,
	requestID string,
	lock bool,
	now time.Time,
) (*entmodel.OIDCAuthorizationRequest, *entmodel.OAuthClient, error) {
	if requestID == "" {
		return nil, nil, biz.ErrNotFound
	}
	query := client.OIDCAuthorizationRequest.Query().
		Where(
			oidcauthorizationrequest.IDEQ(requestID),
			oidcauthorizationrequest.DoneEQ(false),
			oidcauthorizationrequest.ExpiresTimeGT(now),
		)
	if lock {
		query = query.ForUpdate()
	}
	entity, err := query.Only(ctx)
	if err != nil {
		return nil, nil, translateEntError(err)
	}
	oauthClient, err := client.OAuthClient.Get(ctx, entity.ClientID)
	if err != nil {
		return nil, nil, translateEntError(err)
	}
	if oauthClient.Status != biz.ClientStatusActive {
		return nil, nil, biz.ErrNotFound
	}
	return entity, oauthClient, nil
}

func toAuthFlowOIDCContext(client *entmodel.OAuthClient) *authnpb.AuthFlowOidcContext {
	return &authnpb.AuthFlowOidcContext{
		ClientId:   client.ID,
		ClientName: client.DisplayName,
		LogoUri:    derefString(client.LogoURI),
	}
}

func verifyLockedRequiredActionSnapshot(
	ctx context.Context,
	tx *entmodel.Tx,
	current *entmodel.User,
	flowRow *entmodel.AuthFlow,
) (*entmodel.Authenticator, *entmodel.PasswordAuthenticator, bool, error) {
	if current.Status != biz.UserStatusActive {
		return nil, nil, false, nil
	}
	if _, err := ensureVerifiedEmail(ctx, tx, current.ID); err != nil {
		if errors.Is(err, biz.ErrNotFound) || errors.Is(err, biz.ErrMutationMiss) {
			return nil, nil, false, nil
		}
		return nil, nil, false, err
	}
	if flowRow.UserEtag == nil || current.Etag != *flowRow.UserEtag {
		return nil, nil, false, nil
	}
	if !slices.Equal(current.RequiredActions, flowRow.RequiredActions) ||
		len(current.RequiredActions) == 0 ||
		current.RequiredActions[0] != storedRequiredActionUpdatePassword {
		return nil, nil, false, nil
	}
	authEntity, passEntity, err := ensureActivePasswordAuthenticator(ctx, tx, current.ID)
	if err != nil {
		if errors.Is(err, biz.ErrNotFound) {
			return nil, nil, false, nil
		}
		return nil, nil, false, err
	}
	if flowRow.AuthenticatorID == nil || authEntity.ID != *flowRow.AuthenticatorID {
		return nil, nil, false, nil
	}
	if flowRow.PasswordAuthenticatorID == nil || passEntity.ID != *flowRow.PasswordAuthenticatorID {
		return nil, nil, false, nil
	}
	if flowRow.PasswordChangedTime == nil ||
		!biz.NormalizeTimestamp(passEntity.ChangedTime).Equal(biz.NormalizeTimestamp(*flowRow.PasswordChangedTime)) {
		return nil, nil, false, nil
	}
	return authEntity, passEntity, true, nil
}

func isTerminalEntFlowStatus(status authflow.Status) bool {
	switch status {
	case authflow.StatusCompleted, authflow.StatusCancelled, authflow.StatusReset:
		return true
	default:
		return false
	}
}

func toAuthFlowSnapshot(entity *entmodel.AuthFlow) (*biz.AuthFlowSnapshot, error) {
	actions, err := requiredActionsFromStrings(entity.RequiredActions)
	if err != nil {
		return nil, err
	}
	flow := biz.EvaluateFlowProgress(entity.CompletedAmr, biz.DefaultAuthPolicy(), actions)
	flow.FlowId = entity.ID
	flow.ExpiresTime = timestamppb.New(entity.ExpiresTime.UTC())
	switch entity.State {
	case authflow.StateAuthenticate:
		flow.State = authnpb.AuthFlowState_AUTH_FLOW_STATE_AUTHENTICATE
	case authflow.StateRequiredAction:
		flow.State = authnpb.AuthFlowState_AUTH_FLOW_STATE_REQUIRED_ACTION
		flow.Email = derefString(entity.UserEmail)
		if entity.StepExpiresTime != nil {
			flow.StepExpiresTime = timestamppb.New(entity.StepExpiresTime.UTC())
		}
	case authflow.StateCompleted:
		flow.State = authnpb.AuthFlowState_AUTH_FLOW_STATE_COMPLETED
		flow.AvailableMethods = nil
		flow.RemainingRequiredActions = nil
		flow.NextRequiredAction = userpb.UserRequiredAction_USER_REQUIRED_ACTION_UNSPECIFIED
		flow.Email = derefString(entity.UserEmail)
	}
	var pwdChanged *time.Time
	if entity.PasswordChangedTime != nil {
		t := biz.NormalizeTimestamp(*entity.PasswordChangedTime)
		pwdChanged = &t
	}
	return &biz.AuthFlowSnapshot{
		Flow:                    flow,
		OwnerHash:               entity.OwnerHash,
		Status:                  string(entity.Status),
		OIDCRequestID:           derefString(entity.OidcRequestID),
		UserID:                  derefString(entity.UserID),
		UserEtag:                derefString(entity.UserEtag),
		AuthenticatorID:         derefString(entity.AuthenticatorID),
		PasswordAuthenticatorID: derefString(entity.PasswordAuthenticatorID),
		PasswordChangedTime:     pwdChanged,
	}, nil
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
