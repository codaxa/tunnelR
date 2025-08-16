// Package context provides custom types for context keys used in the application.
package context

// Key is a custom type for context keys to avoid collisions
type Key string

// UserClaimsKey is the context key for JWT user claims
const UserClaimsKey Key = "user_claims"
