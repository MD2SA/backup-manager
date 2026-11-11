package s3

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const listPageOne = `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>backups</Name>
  <Prefix>meta/</Prefix>
  <KeyCount>1</KeyCount>
  <MaxKeys>1000</MaxKeys>
  <IsTruncated>true</IsTruncated>
  <NextContinuationToken>page-2</NextContinuationToken>
  <Contents>
    <Key>meta/p1/metadata-20260101-000000.dump.age</Key>
    <LastModified>2026-01-01T00:00:00.000Z</LastModified>
    <ETag>"one"</ETag>
    <Size>8</Size>
    <StorageClass>STANDARD</StorageClass>
  </Contents>
</ListBucketResult>`

const listPageTwo = `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>backups</Name>
  <Prefix>meta/</Prefix>
  <KeyCount>1</KeyCount>
  <MaxKeys>1000</MaxKeys>
  <IsTruncated>false</IsTruncated>
  <Contents>
    <Key>meta/p1/metadata-20260102-000000.dump.age</Key>
    <LastModified>2026-01-02T00:00:00.000Z</LastModified>
    <ETag>"two"</ETag>
    <Size>8</Size>
    <StorageClass>STANDARD</StorageClass>
  </Contents>
</ListBucketResult>`

func TestS3Provider_ListPaginationAndDownload(t *testing.T) {
	const snapshot = "metadata-snapshot-body"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/backups":
			w.Header().Set("Content-Type", "application/xml")
			if r.URL.Query().Get("continuation-token") == "page-2" {
				_, _ = io.WriteString(w, listPageTwo)
				return
			}
			_, _ = io.WriteString(w, listPageOne)
		case r.Method == http.MethodGet && r.URL.Path == "/backups/meta/p1/metadata-20260102-000000.dump.age":
			_, _ = io.WriteString(w, snapshot)
		case r.Method == http.MethodHead && r.URL.Path == "/backups/meta/missing.dump":
			w.WriteHeader(http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := New(context.Background(), Options{
		Region:         "us-east-1",
		Bucket:         "backups",
		AccessKey:      "test",
		SecretKey:      "test",
		Endpoint:       server.URL,
		ForcePathStyle: true,
		UseSSL:         false,
	})
	if err != nil {
		t.Fatal(err)
	}

	keys, err := provider.List(context.Background(), "meta/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 2 ||
		keys[0] != "meta/p1/metadata-20260101-000000.dump.age" ||
		keys[1] != "meta/p1/metadata-20260102-000000.dump.age" {
		t.Fatalf("unexpected keys: %v", keys)
	}

	reader, err := provider.Download(context.Background(), keys[1])
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	body, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatalf("read download: %v", err)
	}
	if strings.TrimSpace(string(body)) != snapshot {
		t.Fatalf("unexpected body: %q", body)
	}

	exists, err := provider.Exists(context.Background(), "meta/missing.dump")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Fatal("expected missing object to report false")
	}
}
