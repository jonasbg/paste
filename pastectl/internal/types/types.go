package types

// Metadata represents file metadata
type Metadata struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

// Config represents server configuration
type Config struct {
	MaxFileSizeBytes int64 `json:"max_file_size_bytes"`
	ChunkSize        int   `json:"chunk_size"`
	KeySize          int   `json:"key_size"`
}
