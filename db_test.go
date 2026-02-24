package main

import "testing"

func TestGetSessionID(t *testing.T) {
	sessionID, err := CreateOrGetSessionID()
	if err != nil || sessionID == 0 {
		t.Fatal(err, sessionID)
	}
}
