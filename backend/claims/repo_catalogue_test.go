package claims

import "testing"

func TestRepoCatalogueIsValid(t *testing.T) {
	c, err := LoadCatalogue("../..")
	if err != nil {
		t.Fatalf("the repo's own catalogue does not load: %v", err)
	}
	if len(c.Checks) == 0 {
		t.Fatal("the repo's catalogue is empty")
	}
	t.Logf("%d checks: %v", len(c.Checks), c.Names())
}
