package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abiiranathan/rex"
	"github.com/golang-jwt/jwt/v5"
)

type claimsType string
type jwtSkipped string

const (
	jwtClaimsKey     claimsType = "claims"
	jwtPayload       string     = "payload"
	expKey           string     = "exp"
	tokenPrefix      string     = "Bearer "
	jwtAuthIsSkipped jwtSkipped = "jwt_auth_skipped"
)

// JWT creates a JWT middleware with the given secret and options.
// If skipFunc returns true, authentication is skipped.
func JWT(secret string, skipFunc func(c *rex.Context) bool) rex.Middleware {
	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(ctx *rex.Context) error {
			if skipFunc != nil && skipFunc(ctx) {
				ctx.Request = ctx.Request.WithContext(context.WithValue(ctx.Request.Context(), jwtAuthIsSkipped, true))
				return next(ctx)
			}

			// Extract the JWT token from the request
			tokenString := ctx.Request.Header.Get("Authorization")

			// Remove the "Bearer " prefix
			tokenString = strings.TrimPrefix(tokenString, tokenPrefix)

			// remove whitespace
			tokenString = strings.TrimSpace(tokenString)

			if tokenString == "" {
				return ctx.WriteHeader(http.StatusUnauthorized)
			}

			// Verify the token
			claims, err := VerifyJWToken(secret, tokenString)
			if err != nil {
				return ctx.WriteHeader(http.StatusUnauthorized)
			}

			ctx.Request = ctx.Request.WithContext(context.WithValue(ctx.Request.Context(), jwtClaimsKey, claims))
			return next(ctx)
		}
	}
}

// CreateToken creates a new JWT token with the given payload and expiry duration.
// JWT is signed with the given secret key using the HMAC256 alegorithm.
func CreateJWTToken(secret string, payload any, exp time.Duration) (string, error) {
	token := jwt.New(jwt.SigningMethodHS256)
	claims := token.Claims.(jwt.MapClaims)
	claims[jwtPayload] = payload
	claims[expKey] = time.Now().Add(exp).Unix()
	return token.SignedString([]byte(secret))
}

// VerifyJWToken verifies the given JWT token with the secret key.
// Returns the claims if the token is valid, otherwise an error.
// The token is verified using the HMAC256 algorithm.
// The default claims are stored in the "payload" key and the expiry time in the "exp" key.
func VerifyJWToken(secret, tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	// Validate the token
	if !token.Valid {
		return nil, fmt.Errorf("invalid or expired token")
	}

	return token.Claims.(jwt.MapClaims), nil
}

// Returns the payload from the request or nil if non-exists.
// Should be called inside the handler when JWT verification is complete.
func GetPayload(req *http.Request) any {
	claims, ok := req.Context().Value(jwtClaimsKey).(jwt.MapClaims)
	if !ok {
		return nil
	}
	return claims[jwtPayload]
}

// Returns true if JWT authentication was skipped.
func JWTAuthSkipped(r *http.Request) bool {
	value := r.Context().Value(jwtAuthIsSkipped)
	if skipped, ok := value.(bool); skipped && ok {
		return true
	}
	return false
}
