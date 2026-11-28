package biz

const (
	LoginIdentifierEmail  = "email"
	AuthenticatorPassword = "password"
	AuthenticatorActive   = "active"
	AuthenticatorDisabled = "disabled"

	UserStatusPendingVerification = "pending_verification"
	UserStatusActive              = "active"
	UserStatusDisabled            = "disabled"

	AMRPassword = "pwd"

	AuthFlowStatusActive      = "active"
	AuthFlowStatusCompleted   = "completed"
	AuthFlowStatusCancelled   = "cancelled"
	AuthFlowStatusReset       = "reset"
	AuthFlowStatusInvalidated = "invalidated"

	ClientSourceStatic  = "static"
	ClientSourceDynamic = "dynamic"

	ClientStatusActive   = "active"
	ClientStatusDisabled = "disabled"

	AppTypeWeb       = "web"
	AppTypeUserAgent = "user_agent"
	AppTypeNative    = "native"

	AuthMethodBasic = "client_secret_basic"
	AuthMethodPost  = "client_secret_post"
	AuthMethodNone  = "none"

	GrantTypeAuthorizationCode = "authorization_code"
	GrantTypeRefreshToken      = "refresh_token"
	GrantTypeClientCredentials = "client_credentials"

	ResponseTypeCode = "code"

	MaxClientSecrets  = 5
	DefaultSecretName = "默认密钥"
)

// SupportedOIDCScopes lists the OIDC user scopes supported by the IAM provider.
var SupportedOIDCScopes = []string{"openid", "profile", "email", "offline_access"}
