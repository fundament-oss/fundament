package catalog

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/dbconst"
	catalogv1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/catalog/v1"
)

// Sort discriminators passed to PluginList. UNSPECIFIED sorts as FEATURED per
// FUN-20, so the storefront's first paint leads with the featured listings.
const (
	sortFeatured      = "featured"
	sortRecentlyAdded = "recently_added"
	sortName          = "name"
)

func sortKey(sort catalogv1.PluginSort) string {
	switch sort {
	case catalogv1.PluginSort_PLUGIN_SORT_UNSPECIFIED, catalogv1.PluginSort_PLUGIN_SORT_FEATURED:
		return sortFeatured
	case catalogv1.PluginSort_PLUGIN_SORT_RECENTLY_ADDED:
		return sortRecentlyAdded
	case catalogv1.PluginSort_PLUGIN_SORT_NAME:
		return sortName
	default:
		panic("unhandled PluginSort: " + sort.String())
	}
}

func labelFromDB(name dbconst.PluginLabelName) catalogv1.PluginLabel {
	switch name {
	case dbconst.PluginLabelName_Core:
		return catalogv1.PluginLabel_PLUGIN_LABEL_CORE
	case dbconst.PluginLabelName_Rijksoverheid:
		return catalogv1.PluginLabel_PLUGIN_LABEL_RIJKSOVERHEID
	case dbconst.PluginLabelName_Support9To17:
		return catalogv1.PluginLabel_PLUGIN_LABEL_SUPPORT_9_TO_17
	default:
		panic("unhandled PluginLabelName: " + string(name))
	}
}

// PluginListRow.Published comes from MIN(timestamptz) in a correlated subquery,
// which sqlc cannot type more precisely than interface{}.
func timestampOrNil(value any) *timestamppb.Timestamp {
	published, ok := value.(time.Time)
	if !ok {
		return nil
	}
	return timestamppb.New(published)
}

// PluginVersionListByPluginIDRow.Published is a direct column select, so sqlc
// types it precisely as pgtype.Timestamptz — unlike the correlated subquery
// above, which sqlc can only type as interface{}.
func timestamptzOrNil(value pgtype.Timestamptz) *timestamppb.Timestamp {
	if !value.Valid {
		return nil
	}
	return timestamppb.New(value.Time)
}
