package whatsapp

import (
	"context"
	"path/filepath"
	"testing"

	"go.mau.fi/whatsmeow/store/sqlstore"
)

func TestSQLiteStore(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "malten.db")
	for i := 0; i < 2; i++ {
		db, err := sqlstore.New(ctx, "sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=foreign_keys(1)", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.GetFirstDevice(ctx)
		closeErr := db.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}
}
