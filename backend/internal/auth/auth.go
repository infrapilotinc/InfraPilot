package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid token")
	ErrTokenExpired       = errors.New("token expired")
	ErrMFARequired        = errors.New("MFA verification required")
)

type Claims struct {
	UserID   uuid.UUID `json:"user_id"`
	OrgID    uuid.UUID `json:"org_id"`
	Email    string    `json:"email"`
	Role     string    `json:"role"`
	MFAValid bool      `json:"mfa_valid"`
	jwt.RegisteredClaims
}

type Service struct {
	jwtSecret []byte
	jwtExpiry time.Duration
}

func NewService(secret string, expiry time.Duration) *Service {
	return &Service{
		jwtSecret: []byte(secret),
		jwtExpiry: expiry,
	}
}

// HashPassword creates a bcrypt hash of the password
func (s *Service) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword checks if the password matches the hash
func (s *Service) VerifyPassword(hash, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateAccessToken creates a new JWT access token
func (s *Service) GenerateAccessToken(userID, orgID uuid.UUID, email, role string, mfaValid bool) (string, error) {
	now := time.Now()
	claims := &Claims{
		UserID:   userID,
		OrgID:    orgID,
		Email:    email,
		Role:     role,
		MFAValid: mfaValid,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.jwtExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "infrapilot",
			Subject:   userID.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// ValidateToken parses and validates a JWT token
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.jwtSecret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// GenerateRefreshToken creates a random refresh token
func (s *Service) GenerateRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateEnrollmentToken creates a random token for agent enrollment
func (s *Service) GenerateEnrollmentToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// Role constants
const (
	RoleSuperAdmin = "super_admin"
	RoleAdmin      = "admin"
	RoleOperator   = "operator"
	RoleViewer     = "viewer"
)

// Permission checks
func CanManageAgents(role string) bool {
	return role == RoleSuperAdmin
}

func CanModifyContainers(role string) bool {
	return role == RoleSuperAdmin || role == RoleOperator
}

func CanExecContainers(role string) bool {
	return role == RoleSuperAdmin || role == RoleOperator
}

func CanModifyProxies(role string) bool {
	return role == RoleSuperAdmin || role == RoleOperator
}

func CanManageAlerts(role string) bool {
	return role == RoleSuperAdmin || role == RoleOperator
}

func CanManageUsers(role string) bool {
	return role == RoleSuperAdmin
}
