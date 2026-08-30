package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrorType categorizes a database failure into a class a repository can
// branch on, independent of the underlying driver's error type.
type ErrorType string

// Error classes recognized by Classify.
const (
	ErrorNone             ErrorType = ""
	ErrorNotFound         ErrorType = "not_found"
	ErrorUniqueViolation  ErrorType = "unique_violation"
	ErrorForeignKey       ErrorType = "foreign_key_violation"
	ErrorCheckViolation   ErrorType = "check_violation"
	ErrorConnection       ErrorType = "connection_error"
	ErrorInvalidInput     ErrorType = "invalid_input"
	ErrorPermissionDenied ErrorType = "permission_denied"
	ErrorBusiness         ErrorType = "business_error"
	ErrorUnknown          ErrorType = "unknown"
)

// Classify maps err to an ErrorType and a human-readable detail message.
//
// Usage:
//
//	if errType, detail := database.Classify(err); errType == database.ErrorUniqueViolation {
//	    return response.New(response.Conflict, "email_already_exists")
//	}
func Classify(err error) (ErrorType, string) {
	if err == nil {
		return ErrorNone, ""
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrorNotFound, "resource not found"
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return classifyPgError(pgErr)
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrorConnection, "query timeout or canceled"
	}

	return ErrorUnknown, err.Error()
}

// classifyPgError maps a Postgres SQLSTATE code to an ErrorType.
// See https://www.postgresql.org/docs/current/errcodes-appendix.html.
func classifyPgError(pgErr *pgconn.PgError) (ErrorType, string) {
	switch pgErr.Code {
	case "P0001": // raise_exception
		return ErrorBusiness, pgErr.Message
	case "23505": // unique_violation
		return ErrorUniqueViolation, fmt.Sprintf("duplicate value violates unique constraint (%s)", pgErr.ConstraintName)
	case "23503": // foreign_key_violation
		return ErrorForeignKey, fmt.Sprintf("foreign key constraint violation (%s)", pgErr.ConstraintName)
	case "23514": // check_violation
		return ErrorCheckViolation, fmt.Sprintf("check constraint violation (%s)", pgErr.ConstraintName)
	case "28P01", "42501": // invalid_password, insufficient_privilege
		return ErrorPermissionDenied, "permission denied"
	case "08000", "08001", "08003", "08004", "08006", "08P01":
		return ErrorConnection, "database connection error"
	case "22P02", "22007", "22008": // invalid_text_representation, invalid_datetime
		return ErrorInvalidInput, "invalid input format"
	default:
		return ErrorUnknown, fmt.Sprintf("database error (%s): %s", pgErr.Code, pgErr.Message)
	}
}

// IsNotFound reports whether err represents a missing row (pgx.ErrNoRows).
func IsNotFound(err error) bool {
	t, _ := Classify(err)
	return t == ErrorNotFound
}

// IsUniqueViolation reports whether err represents a unique-constraint violation.
func IsUniqueViolation(err error) bool {
	t, _ := Classify(err)
	return t == ErrorUniqueViolation
}

// IsForeignKeyViolation reports whether err represents a foreign-key constraint violation.
func IsForeignKeyViolation(err error) bool {
	t, _ := Classify(err)
	return t == ErrorForeignKey
}

// IsCheckViolation reports whether err represents a check-constraint violation.
func IsCheckViolation(err error) bool {
	t, _ := Classify(err)
	return t == ErrorCheckViolation
}

// IsConnectionError reports whether err represents a connectivity or timeout failure.
func IsConnectionError(err error) bool {
	t, _ := Classify(err)
	return t == ErrorConnection
}

// Match translates a database error into a domain error using the provided mapping.
//
// Usage:
//
//	err = database.Match(err, map[database.ErrorType]error{
//	    database.ErrorUniqueViolation: domain.ErrEmailAlreadyExists,
//	    database.ErrorNotFound:        domain.ErrUserNotFound,
//	})
func Match(err error, mapping map[ErrorType]error) error {
	if err == nil {
		return nil
	}

	errType, _ := Classify(err)
	if mapped, ok := mapping[errType]; ok {
		return mapped
	}
	return err
}
