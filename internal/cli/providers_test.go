package cli

import "testing"

func TestProvidersCommandExposesExpectedSubcommands(t *testing.T) {
	cmd := newProvidersCmd()

	got := make([]string, 0, len(cmd.Commands()))
	for _, subcmd := range cmd.Commands() {
		got = append(got, subcmd.Name())
	}

	want := []string{"sync", "update"}
	if len(got) != len(want) {
		t.Fatalf("subcommand count = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("subcommands = %v, want %v", got, want)
		}
	}
}
