package main

import "testing"

func TestUpdateInitializationUsesExistingJournalOnly(t *testing.T) {
	directory, err := parseUpdateInitialization(nil)
	if err != nil || directory != "/var/lib/homenode-install" {
		t.Fatal(directory, err)
	}
	for _, args := range [][]string{{"--journal-dir", "relative"}, {"--journal-dir", "/var/lib/../state"}, {"--root", "/other"}, {"--bootstrap", "untrusted"}, {"unexpected"}} {
		if _, err = parseUpdateInitialization(args); err == nil {
			t.Fatal("unowned initialization option accepted", args)
		}
	}
}
