package toolcall

import (
	"testing"
)

func TestGetSessionID(t *testing.T) {
	ctx := t.Context()
	sessionID, err := CreateOrGetSessionID(ctx)
	if err != nil || sessionID == 0 {
		t.Fatal(err, sessionID)
	}
}
