package client

import "testing"

func TestFnoName(t *testing.T) {
	ltox := NewLigTox()
	ltox.FriendSendMessage(4, "aa")
}
