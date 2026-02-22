package db

import "testing"

func TestMediaRemarkPersisted(t *testing.T) {
	db := openTestDB(t)

	item := &Media{
		Kind:              MediaKindImage,
		Provider:          "see",
		ProviderAssetID:   uniqueValue("asset"),
		ProviderDeleteKey: "delete-key",
		FileURL:           "https://example.com/file.png",
		OriginalName:      "file.png",
		Remark:            "  manual note  ",
		SizeBytes:         1024,
	}

	id, err := CreateMedia(db, item)
	if err != nil {
		t.Fatalf("CreateMedia failed: %v", err)
	}

	got, err := GetMediaByID(db, id)
	if err != nil {
		t.Fatalf("GetMediaByID failed: %v", err)
	}
	if got.Remark != "manual note" {
		t.Fatalf("unexpected remark: got %q want %q", got.Remark, "manual note")
	}

	list, err := ListMedia(db, MediaQueryOptions{Limit: 20, Offset: 0})
	if err != nil {
		t.Fatalf("ListMedia failed: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("ListMedia returned empty result")
	}
	if list[0].Remark != "manual note" {
		t.Fatalf("unexpected list remark: got %q want %q", list[0].Remark, "manual note")
	}
}
