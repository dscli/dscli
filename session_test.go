package main

import (
	"testing"

	"gitcode.com/dscli/dscli/internal/context"
)

func TestGetSessionID(t *testing.T) {
	ctx := t.Context()
	ctx = context.WithValue(ctx, context.ProjectRootKey, context.GetProjectRoot())
	sessionID, err := CreateOrGetSessionID(ctx)
	if err != nil || sessionID == 0 {
		t.Fatal(err, sessionID)
	}
}
