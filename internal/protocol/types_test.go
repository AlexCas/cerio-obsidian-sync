package protocol

import (
	"encoding/json"
	"testing"
	"time"
)

// roundTrip marshals v to JSON and unmarshals back into target.
// Returns an error if marshaling or unmarshaling fails.
func roundTrip(t *testing.T, v interface{}, target interface{}) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

func TestSyncSessionJSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	original := SyncSession{
		SessionID: "sess-123",
		VaultID:   "vault-abc",
		Timestamp: now,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal SyncSession: %v", err)
	}

	var decoded SyncSession
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal SyncSession: %v", err)
	}

	if decoded.SessionID != original.SessionID {
		t.Errorf("SessionID = %q, want %q", decoded.SessionID, original.SessionID)
	}
	if decoded.VaultID != original.VaultID {
		t.Errorf("VaultID = %q, want %q", decoded.VaultID, original.VaultID)
	}
}

func TestFileEntryJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		entry   FileEntry
		wantErr bool
	}{
		{
			name: "complete entry",
			entry: FileEntry{
				Path:       "notes/daily/2024-01-15.md",
				Hash:       "abc123def456",
				Size:       2048,
				ModifiedAt: "2024-01-15T10:30:00Z",
			},
			wantErr: false,
		},
		{
			name: "empty path",
			entry: FileEntry{
				Path:       "",
				Hash:       "e3b0c44298fc1c149afbf4c8996fb924",
				Size:       0,
				ModifiedAt: "2024-01-01T00:00:00Z",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.entry)
			if (err != nil) != tt.wantErr {
				t.Fatalf("marshal: err=%v, wantErr=%v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			var decoded FileEntry
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if decoded.Path != tt.entry.Path {
				t.Errorf("Path = %q, want %q", decoded.Path, tt.entry.Path)
			}
			if decoded.Hash != tt.entry.Hash {
				t.Errorf("Hash = %q, want %q", decoded.Hash, tt.entry.Hash)
			}
			if decoded.Size != tt.entry.Size {
				t.Errorf("Size = %d, want %d", decoded.Size, tt.entry.Size)
			}
			if decoded.ModifiedAt != tt.entry.ModifiedAt {
				t.Errorf("ModifiedAt = %q, want %q", decoded.ModifiedAt, tt.entry.ModifiedAt)
			}
		})
	}
}

func TestManifestJSONRoundTrip(t *testing.T) {
	original := Manifest{
		Entries: []FileEntry{
			{Path: "a.md", Hash: "hash_a", Size: 100, ModifiedAt: "2024-01-01T00:00:00Z"},
			{Path: "b.md", Hash: "hash_b", Size: 200, ModifiedAt: "2024-01-02T00:00:00Z"},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal Manifest: %v", err)
	}

	var decoded Manifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal Manifest: %v", err)
	}

	if len(decoded.Entries) != len(original.Entries) {
		t.Fatalf("Entries length = %d, want %d", len(decoded.Entries), len(original.Entries))
	}
	for i, got := range decoded.Entries {
		want := original.Entries[i]
		if got.Path != want.Path {
			t.Errorf("Entries[%d].Path = %q, want %q", i, got.Path, want.Path)
		}
		if got.Hash != want.Hash {
			t.Errorf("Entries[%d].Hash = %q, want %q", i, got.Hash, want.Hash)
		}
	}
}

func TestSyncBeginRequestJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		req     SyncBeginRequest
		wantErr bool
	}{
		{
			name: "incremental sync",
			req: SyncBeginRequest{
				VaultID:       "vault-1",
				ClientID:      "client-a",
				SinceRevision: 42,
			},
		},
		{
			name: "initial sync with zero revision",
			req: SyncBeginRequest{
				VaultID:       "vault-1",
				ClientID:      "client-b",
				SinceRevision: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("marshal: err=%v, wantErr=%v", err, tt.wantErr)
			}
			var decoded SyncBeginRequest
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if decoded.VaultID != tt.req.VaultID {
				t.Errorf("VaultID = %q, want %q", decoded.VaultID, tt.req.VaultID)
			}
			if decoded.ClientID != tt.req.ClientID {
				t.Errorf("ClientID = %q, want %q", decoded.ClientID, tt.req.ClientID)
			}
			if decoded.SinceRevision != tt.req.SinceRevision {
				t.Errorf("SinceRevision = %d, want %d", decoded.SinceRevision, tt.req.SinceRevision)
			}
		})
	}
}

func TestSyncBeginResponseJSONRoundTrip(t *testing.T) {
	original := SyncBeginResponse{
		ServerChanges: []FileChange{
			{Path: "a.md", Hash: "h1", ModifiedAt: "2024-01-01T00:00:00Z", Size: 100, Operation: OperationCreate},
			{Path: "b.md", Hash: "h2", ModifiedAt: "2024-01-02T00:00:00Z", Size: 200, Operation: OperationUpdate},
			{Path: "c.md", Hash: "", ModifiedAt: "2024-01-03T00:00:00Z", Size: 0, Operation: OperationDelete},
		},
		Revision: 43,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded SyncBeginResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Revision != original.Revision {
		t.Errorf("Revision = %d, want %d", decoded.Revision, original.Revision)
	}
	if len(decoded.ServerChanges) != len(original.ServerChanges) {
		t.Fatalf("ServerChanges length = %d, want %d", len(decoded.ServerChanges), len(original.ServerChanges))
	}
	for i, got := range decoded.ServerChanges {
		want := original.ServerChanges[i]
		if got.Path != want.Path {
			t.Errorf("ServerChanges[%d].Path = %q, want %q", i, got.Path, want.Path)
		}
		if got.Operation != want.Operation {
			t.Errorf("ServerChanges[%d].Operation = %q, want %q", i, got.Operation, want.Operation)
		}
	}
}

func TestManifestResponseJSONRoundTrip(t *testing.T) {
	original := ManifestResponse{
		Need:     []string{"file1.md", "file2.md"},
		Have:     []string{"file3.md"},
		Conflict: []string{"file4.md", "file5.md"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ManifestResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(decoded.Need) != len(original.Need) {
		t.Errorf("Need length = %d, want %d", len(decoded.Need), len(original.Need))
	}
	if len(decoded.Have) != len(original.Have) {
		t.Errorf("Have length = %d, want %d", len(decoded.Have), len(original.Have))
	}
	if len(decoded.Conflict) != len(original.Conflict) {
		t.Errorf("Conflict length = %d, want %d", len(decoded.Conflict), len(original.Conflict))
	}
}

func TestFileUploadRequestJSONRoundTrip(t *testing.T) {
	original := FileUploadRequest{
		Path: "notes/test.md",
		Hash: "sha256hashvalue",
		Size: 4096,
	}

	var decoded FileUploadRequest
	roundTrip(t, &original, &decoded)

	if decoded.Path != original.Path {
		t.Errorf("Path = %q, want %q", decoded.Path, original.Path)
	}
	if decoded.Hash != original.Hash {
		t.Errorf("Hash = %q, want %q", decoded.Hash, original.Hash)
	}
	if decoded.Size != original.Size {
		t.Errorf("Size = %d, want %d", decoded.Size, original.Size)
	}
}

func TestFileDownloadResponseJSONRoundTrip(t *testing.T) {
	original := FileDownloadResponse{
		Path:       "notes/download.md",
		Hash:       "downloadhash",
		Size:       8192,
		ModifiedAt: "2024-01-15T12:00:00Z",
		Content:    []byte("file content here"),
	}

	var decoded FileDownloadResponse
	roundTrip(t, &original, &decoded)

	if decoded.Path != original.Path {
		t.Errorf("Path = %q, want %q", decoded.Path, original.Path)
	}
	if decoded.Size != original.Size {
		t.Errorf("Size = %d, want %d", decoded.Size, original.Size)
	}
	if string(decoded.Content) != string(original.Content) {
		t.Errorf("Content = %q, want %q", string(decoded.Content), string(original.Content))
	}
}

func TestErrorResponseJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		err  ErrorResponse
	}{
		{
			name: "unauthorized",
			err:  ErrorResponse{Code: ErrCodeUnauthorized, Message: "invalid API key"},
		},
		{
			name: "not found",
			err:  ErrorResponse{Code: ErrCodeNotFound, Message: "file not found"},
		},
		{
			name: "payload too large",
			err:  ErrorResponse{Code: ErrCodePayloadTooLarge, Message: "file exceeds 50MB limit"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var decoded ErrorResponse
			roundTrip(t, &tt.err, &decoded)

			if decoded.Code != tt.err.Code {
				t.Errorf("Code = %q, want %q", decoded.Code, tt.err.Code)
			}
			if decoded.Message != tt.err.Message {
				t.Errorf("Message = %q, want %q", decoded.Message, tt.err.Message)
			}
		})
	}
}

func TestSyncCompleteRequestJSONRoundTrip(t *testing.T) {
	original := SyncCompleteRequest{Revision: 99}

	var decoded SyncCompleteRequest
	roundTrip(t, &original, &decoded)

	if decoded.Revision != original.Revision {
		t.Errorf("Revision = %d, want %d", decoded.Revision, original.Revision)
	}
}

func TestManifestEntryIsFileEntryAlias(t *testing.T) {
	// ManifestEntry is a type alias for FileEntry — both should be assignable.
	entry := FileEntry{
		Path:       "test.md",
		Hash:       "abc",
		Size:       42,
		ModifiedAt: "2024-01-01T00:00:00Z",
	}
	var manifestEntry ManifestEntry = entry

	if manifestEntry.Path != entry.Path {
		t.Errorf("ManifestEntry.Path = %q, want %q", manifestEntry.Path, entry.Path)
	}
	if manifestEntry.Hash != entry.Hash {
		t.Errorf("ManifestEntry.Hash = %q, want %q", manifestEntry.Hash, entry.Hash)
	}
}

func TestSyncRequestResponseAliases(t *testing.T) {
	// SyncRequest and SyncResponse are aliases for SyncBeginRequest and SyncBeginResponse.
	var _ SyncRequest = SyncBeginRequest{}
	var _ SyncResponse = SyncBeginResponse{}
}

func TestConstants(t *testing.T) {
	tests := []struct {
		name  string
		got   interface{}
		want  interface{}
	}{
		{"APIV1Base", APIV1Base, "/api/v1"},
		{"SyncBeginPath", SyncBeginPath, "/api/v1/sync/begin"},
		{"SyncManifestPath", SyncManifestPath, "/api/v1/sync/manifest"},
		{"SyncFilePath", SyncFilePath, "/api/v1/sync/file"},
		{"SyncCompletePath", SyncCompletePath, "/api/v1/sync/complete"},
		{"MaxFileSize", MaxFileSize, int64(50 * 1024 * 1024)},
		{"DefaultPort", DefaultPort, 8080},
		{"DefaultPageSize", DefaultPageSize, 5000},
		{"SessionTimeout", SessionTimeout, 10 * time.Minute},
		{"SoftDeleteGraceDays", SoftDeleteGraceDays, 30},
		{"OperationCreate", OperationCreate, "create"},
		{"OperationUpdate", OperationUpdate, "update"},
		{"OperationDelete", OperationDelete, "delete"},
		{"ErrCodeUnauthorized", ErrCodeUnauthorized, "unauthorized"},
		{"ErrCodePayloadTooLarge", ErrCodePayloadTooLarge, "payload_too_large"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}