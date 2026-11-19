package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
)

func testCategories() []*marketplacev1.Category {
	return []*marketplacev1.Category{
		marketplacev1.Category_builder{Id: "id-security", Name: "Security"}.Build(),
		marketplacev1.Category_builder{Id: "id-networking", Name: "Networking"}.Build(),
		marketplacev1.Category_builder{Id: "id-database", Name: "Database"}.Build(),
	}
}

func TestMatchCategories(t *testing.T) {
	ids, err := matchCategories([]string{"security", " Networking "}, testCategories())
	require.NoError(t, err)
	assert.Equal(t, []string{"id-security", "id-networking"}, ids)
}

// The registry inserts one row per id, so a name given twice in another case
// must not reach it twice.
func TestMatchCategoriesDeduplicates(t *testing.T) {
	ids, err := matchCategories([]string{"Security", "SECURITY"}, testCategories())
	require.NoError(t, err)
	assert.Equal(t, []string{"id-security"}, ids)
}

// A typo has to stop the publish: otherwise the plugin silently ends up under
// Other and nobody notices.
func TestMatchCategoriesRejectsUnknownNames(t *testing.T) {
	_, err := matchCategories([]string{"Security", "Sercurity", "Monitoring"}, testCategories())
	require.Error(t, err)
	assert.EqualError(t, err,
		`unknown metadata.categories "Sercurity", "Monitoring"; expected any of: Database, Networking, Security`)
}

func TestMatchCategoriesRejectsTooMany(t *testing.T) {
	categories := make([]*marketplacev1.Category, 0, maxCategories+1)
	names := make([]string, 0, maxCategories+1)
	for i := range maxCategories + 1 {
		name := string(rune('a' + i))
		categories = append(categories, marketplacev1.Category_builder{Id: "id-" + name, Name: name}.Build())
		names = append(names, name)
	}

	_, err := matchCategories(names, categories)
	assert.ErrorContains(t, err, "at most 10 are allowed")
}
