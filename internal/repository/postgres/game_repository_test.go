package postgres

import "testing"

func TestBuildWalletAddressVariants_RawAddressIncludesFriendlyVariants(t *testing.T) {
	t.Parallel()

	rawAddress := "0:4976439397a8d8db3bb07ca725a8798c313d41ce305815a1b7c1752132d3dcac"
	expectedFriendlyAddress := "EQBJdkOTl6jY2zuwfKclqHmMMT1BzjBYFaG3wXUhMtPcrAzX"

	variants := buildWalletAddressVariants(rawAddress)
	if len(variants) == 0 {
		t.Fatal("expected at least one wallet variant")
	}

	contains := func(target string) bool {
		for _, variant := range variants {
			if variant == target {
				return true
			}
		}

		return false
	}

	if !contains(rawAddress) {
		t.Fatalf("expected raw wallet variant %q in %v", rawAddress, variants)
	}

	if !contains(expectedFriendlyAddress) {
		t.Fatalf("expected friendly wallet variant %q in %v", expectedFriendlyAddress, variants)
	}
}
