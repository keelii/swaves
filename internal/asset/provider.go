package asset

import "context"

type UploadInput struct {
	FileName    string
	ContentType string
	Bytes       []byte
}

type UploadResult struct {
	ProviderAssetID   string
	ProviderDeleteKey string
	FileURL           string
	OriginalName      string
	SizeBytes         int64
}

type ListInput struct {
	Page     int
	PageSize int
}

type ListItem struct {
	ProviderAssetID   string
	ProviderDeleteKey string
	FileURL           string
	OriginalName      string
	SizeBytes         int64
	CreatedAt         int64
}

type ListResult struct {
	Items []ListItem
}

type Provider interface {
	Name() string
	Upload(ctx context.Context, in UploadInput) (*UploadResult, error)
	List(ctx context.Context, in ListInput) (*ListResult, error)
	Delete(ctx context.Context, deleteKey string) error
}
