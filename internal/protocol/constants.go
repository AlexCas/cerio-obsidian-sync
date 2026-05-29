package protocol

import "time"

// API path constants.
const (
	// API base path for v1 endpoints.
	APIV1Base = "/api/v1"

	// Sync endpoints.
	SyncBeginPath    = "/api/v1/sync/begin"
	SyncManifestPath = "/api/v1/sync/manifest"
	SyncFilePath    = "/api/v1/sync/file"
	SyncCompletePath = "/api/v1/sync/complete"

	// Health check endpoint.
	HealthPath = "/api/v1/health"
)

// Timeout and limit constants.
const (
	// MaxFileSize is the maximum allowed file upload size (50 MB).
	MaxFileSize int64 = 50 * 1024 * 1024

	// DefaultPort is the default server listen port.
	DefaultPort = 8080

	// DefaultPageSize is the default number of entries per manifest page.
	DefaultPageSize = 5000

	// PaginationThreshold is the number of manifest entries above which
	// the client sends the manifest in paginated requests.
	PaginationThreshold = 10000

	// SessionTimeout is how long a sync session may remain idle before rollback.
	SessionTimeout = 10 * time.Minute

	// SoftDeleteGraceDays is the number of days a soft-deleted file is retained.
	SoftDeleteGraceDays = 30

	// DebounceInterval is the default fsnotify debounce window.
	DebounceInterval = 500 * time.Millisecond

	// SafetyScanInterval is the default periodic full-vault scan interval.
	SafetyScanInterval = 60 * time.Second

	// SyncInterval is the default interval between automatic sync cycles.
	SyncInterval = 30 * time.Second

	// WatcherBufferSize is the fsnotify event buffer size for burst protection.
	WatcherBufferSize = 4096

	// JournalOverflowWarning is the threshold above which a full resync is suggested.
	JournalOverflowWarning = 10000
)

// Operation types used in FileChange.Operation.
const (
	OperationCreate = "create"
	OperationUpdate = "update"
	OperationDelete = "delete"
)

// Error codes used in ErrorResponse.Code.
const (
	ErrCodeUnauthorized    = "unauthorized"
	ErrCodePayloadTooLarge = "payload_too_large"
	ErrCodeNotFound        = "not_found"
	ErrCodeConflict        = "conflict"
	ErrCodeBadRequest      = "bad_request"
	ErrCodeInternal        = "internal_error"
)