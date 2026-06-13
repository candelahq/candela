package catalog_test

import (
	"testing"

	"github.com/candelahq/candela/pkg/catalog"
)

func TestConfigStore(t *testing.T) {
	store := catalog.NewConfigStore(testEntries)
	runConformanceSuite(t, store, false)
}
