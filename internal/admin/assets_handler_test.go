package admin

import (
	"testing"

	"swaves/internal/asset"
	"swaves/internal/db"
)

func TestNormalizeAssetIDs(t *testing.T) {
	got := normalizeAssetIDs([]int64{0, -1, 4, 4, 9, 3, 9})
	want := []int64{4, 9, 3}
	if len(got) != len(want) {
		t.Fatalf("normalizeAssetIDs length = %d, want %d", len(got), len(want))
	}
	for idx, item := range want {
		if got[idx] != item {
			t.Fatalf("normalizeAssetIDs[%d] = %d, want %d", idx, got[idx], item)
		}
	}
}

func TestNormalizeAssetIDsAllInvalid(t *testing.T) {
	got := normalizeAssetIDs([]int64{-4, 0, -2})
	if got != nil {
		t.Fatalf("normalizeAssetIDs should return nil for invalid ids, got=%v", got)
	}
}

func TestDetectAssetKindByUploadedSuffix(t *testing.T) {
	tests := []struct {
		name     string
		uploaded *asset.UploadResult
		fallback string
		want     string
	}{
		{
			name: "image by uploaded original name",
			uploaded: &asset.UploadResult{
				OriginalName: "avatar.JPG",
				FileURL:      "https://cdn.example.com/files/a.bin",
			},
			fallback: "raw.dat",
			want:     db.AssetKindImage,
		},
		{
			name: "file by uploaded original name",
			uploaded: &asset.UploadResult{
				OriginalName: "report.pdf",
				FileURL:      "https://cdn.example.com/files/report.pdf",
			},
			fallback: "x.png",
			want:     db.AssetKindFile,
		},
		{
			name: "image by uploaded url suffix",
			uploaded: &asset.UploadResult{
				OriginalName: "upload_without_ext",
				FileURL:      "https://cdn.example.com/uploads/cover.webp?x-oss-process=image/resize",
			},
			fallback: "backup.bin",
			want:     db.AssetKindImage,
		},
		{
			name: "image by fallback name when upload fields have no suffix",
			uploaded: &asset.UploadResult{
				OriginalName: "server-file",
				FileURL:      "https://cdn.example.com/file/no-ext",
			},
			fallback: "local.png",
			want:     db.AssetKindImage,
		},
		{
			name: "default to file when no suffix available",
			uploaded: &asset.UploadResult{
				OriginalName: "server-file",
				FileURL:      "https://cdn.example.com/file/no-ext",
			},
			fallback: "local-file",
			want:     db.AssetKindFile,
		},
		{
			name:     "nil upload still uses fallback suffix",
			uploaded: nil,
			fallback: "fallback.gif",
			want:     db.AssetKindImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectAssetKindByUploadedSuffix(tt.uploaded, tt.fallback)
			if got != tt.want {
				t.Fatalf("detectAssetKindByUploadedSuffix() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestNormalizedAssetSuffix(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: ""},
		{name: "plain name", raw: "photo.JPEG", want: "jpeg"},
		{name: "url path with query", raw: "https://cdn.example.com/a/b/c.svg?token=1", want: "svg"},
		{name: "url without suffix", raw: "https://cdn.example.com/path/noext", want: ""},
		{name: "relative path with fragment", raw: "/x/y/report.tar.gz#anchor", want: "gz"},
		{name: "dotfile keeps suffix body", raw: ".gitignore", want: "gitignore"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizedAssetSuffix(tt.raw)
			if got != tt.want {
				t.Fatalf("normalizedAssetSuffix() = %q, want %q", got, tt.want)
			}
		})
	}
}
