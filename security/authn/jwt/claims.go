// Package jwt provides Bearer JWT authentication and shared Plateau OAuth access token claims.
package jwt

import (
	"fmt"

	security "github.com/Servora-Kit/plateau/security"
	"github.com/golang-jwt/jwt/v5"
)

const accessTokenUse = "access"

// AccessClaims carries the shared Plateau OAuth 2.1 access token claims.
type AccessClaims struct {
	jwt.RegisteredClaims
	ClientID  string             `json:"client_id"`
	TokenUse  string             `json:"token_use"`
	ActorType security.ActorType `json:"actor_type"`
}

// Validate enforces the shared access token invariants (token_use=access, iat, jti).
func (c *AccessClaims) Validate() error {
	if c == nil {
		return fmt.Errorf("jwt authn: access claims are nil")
	}
	if c.TokenUse != accessTokenUse {
		return fmt.Errorf("jwt authn: unexpected token_use %q", c.TokenUse)
	}
	if c.IssuedAt == nil || c.ID == "" {
		return fmt.Errorf("jwt authn: access token is missing issuance claims")
	}
	return nil
}

// ServiceClaims represents an IAM client_credentials service access token.
type ServiceClaims struct {
	AccessClaims
}

// Validate enforces service access token invariants on top of AccessClaims.
func (c *ServiceClaims) Validate() error {
	if c == nil {
		return fmt.Errorf("jwt authn: service claims are nil")
	}
	if err := c.AccessClaims.Validate(); err != nil {
		return err
	}
	if c.ActorType != security.ActorTypeService || c.ClientID == "" || c.Subject != c.ClientID {
		return fmt.Errorf("jwt authn: service access token requires actor_type=service and matching non-empty sub/client_id")
	}
	return nil
}

// Actor maps verified ServiceClaims to a service Actor.
func (c *ServiceClaims) Actor() (security.Actor, error) {
	if err := c.Validate(); err != nil {
		return security.Actor{}, err
	}
	return security.Actor{Type: security.ActorTypeService, ID: c.ClientID}, nil
}

// HumanClaims represents an IAM user access token.
type HumanClaims struct {
	AccessClaims
}

// Validate enforces human access token invariants on top of AccessClaims.
func (c *HumanClaims) Validate() error {
	if c == nil {
		return fmt.Errorf("jwt authn: human claims are nil")
	}
	if err := c.AccessClaims.Validate(); err != nil {
		return err
	}
	if c.ActorType != security.ActorTypeHuman || c.Subject == "" {
		return fmt.Errorf("jwt authn: human access token requires actor_type=human and non-empty sub")
	}
	return nil
}

// Actor maps verified HumanClaims to a human Actor.
func (c *HumanClaims) Actor() (security.Actor, error) {
	if err := c.Validate(); err != nil {
		return security.Actor{}, err
	}
	return security.Actor{Type: security.ActorTypeHuman, ID: c.Subject}, nil
}
