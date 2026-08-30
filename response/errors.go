package response

// Sentinel errors for cross-cutting failures common to every feature.
var (
	// ErrInternalServer is the generic mapping for untraced, unexpected failures.
	ErrInternalServer = New(Internal, "internal_server_error")

	// ErrDatabaseConnection marks a failure to reach or use the database.
	ErrDatabaseConnection = New(Internal, "database_connection_error")
)
