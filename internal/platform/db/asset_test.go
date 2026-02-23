package db

import "testing"

func TestAssetRemarkPersisted(t *testing.T) {
	db := openTestDB(t)

	item := &Asset{
		Kind:              AssetKindImage,
		Provider:          "see",
		ProviderAssetID:   uniqueValue("asset"),
		ProviderDeleteKey: "delete-key",
		FileURL:           "https://example.com/file.png",
		OriginalName:      "file.png",
		Remark:            "  manual note  ",
		SizeBytes:         1024,
	}

	id, err := CreateAsset(db, item)
	if err != nil {
		t.Fatalf("CreateAsset failed: %v", err)
	}

	got, err := GetAssetByID(db, id)
	if err != nil {
		t.Fatalf("GetAssetByID failed: %v", err)
	}
	if got.Remark != "manual note" {
		t.Fatalf("unexpected remark: got %q want %q", got.Remark, "manual note")
	}

	list, err := ListAssets(db, AssetQueryOptions{Limit: 20, Offset: 0})
	if err != nil {
		t.Fatalf("ListAssets failed: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("ListAssets returned empty result")
	}
	if list[0].Remark != "manual note" {
		t.Fatalf("unexpected list remark: got %q want %q", list[0].Remark, "manual note")
	}
}
