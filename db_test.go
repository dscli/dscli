package main

import "testing"

func TestGetSessionID(t *testing.T) {
	sessionID := GetSessionID()
	if sessionID == 0 {
		t.Fatal(sessionID)
	}
}
