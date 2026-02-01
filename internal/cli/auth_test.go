package cli

import "testing"

func TestAuthCommandStructure(t *testing.T) {
	if AuthCommand == nil {
		t.Fatal("AuthCommand is nil")
	}

	if AuthCommand.Name != "auth" {
		t.Errorf("Expected command name 'auth', got '%s'", AuthCommand.Name)
	}

	if AuthCommand.Usage == "" {
		t.Error("AuthCommand should have usage text")
	}

	if len(AuthCommand.Commands) == 0 {
		t.Fatal("AuthCommand should have subcommands")
	}

	var hasNewKey bool
	for _, subcmd := range AuthCommand.Commands {
		if subcmd.Name != "new-key" {
			continue
		}
		hasNewKey = true
		if subcmd.Usage == "" {
			t.Error("AuthNewKeyCommand should have usage text")
		}
		if subcmd.Description == "" {
			t.Error("AuthNewKeyCommand should have description")
		}
		if subcmd.Action == nil {
			t.Error("AuthNewKeyCommand should have action function")
		}
		break
	}

	if !hasNewKey {
		t.Error("AuthCommand should have 'new-key' subcommand")
	}
}
