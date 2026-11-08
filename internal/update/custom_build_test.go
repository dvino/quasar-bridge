package update

import (
	"context"
	"strings"
	"testing"
)

func TestCustomBuildRejectsOfficialInstall(t *testing.T) {
	u := New("config.json")
	_, err := u.Install(context.Background())
	if err == nil || !strings.Contains(err.Error(), CustomBuildUpdateReason) {
		t.Fatalf("Install() error = %v, want custom build protection", err)
	}
}
