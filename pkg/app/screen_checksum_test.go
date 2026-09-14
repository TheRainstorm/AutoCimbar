package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScreenChecksumPublishedDuringPreparation(t *testing.T) {
	for _, content := range []string{"first file", "different file"} {
		path := filepath.Join(t.TempDir(), "input.bin")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		got := ""
		_, err := EncodeFileToScreen(ScreenEncodeConfig{
			InputPath: path, GridSize: 20, Scale: 1, ECCPercent: 3, ColorBits: 2,
			// Fail before opening a screen window, after preparing source data.
			Region:     "invalid:region:field:count",
			OnChecksum: func(md5 string) { got = md5 },
		})
		if err == nil || !strings.Contains(err.Error(), "region") {
			t.Fatalf("expected region error, got %v", err)
		}
		if got != BytesMD5Hex([]byte(content)) {
			t.Fatalf("checksum not published during preparation: %q", got)
		}
	}
}
