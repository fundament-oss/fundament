package dcim

import (
	"math"
	"math/big"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func numericToFloat64(n pgtype.Numeric) float64 {
	f, _ := n.Float64Value()
	return f.Float64
}

func float64ToNumeric(f float64) pgtype.Numeric {
	// Convert float64 to pgtype.Numeric via integer representation
	// Multiply by 10^6 to preserve 6 decimal places, then set exponent to -6
	const scale = 6
	multiplier := math.Pow10(scale)
	intVal := new(big.Int)
	intVal.SetInt64(int64(math.Round(f * multiplier)))
	return pgtype.Numeric{
		Int:   intVal,
		Exp:   -scale,
		Valid: true,
	}
}

func layoutFromRow(row *db.LogicalDeviceLayoutGetByDesignRow) *dcimv1.LogicalDeviceLayout {
	return dcimv1.LogicalDeviceLayout_builder{
		DeviceId:  row.LogicalDeviceID.String(),
		PositionX: numericToFloat64(row.PositionX),
		PositionY: numericToFloat64(row.PositionY),
		Updated:   timestamppb.New(row.Updated.Time),
	}.Build()
}

func layoutFromUpsertRow(row *db.LogicalDeviceLayoutUpsertRow) *dcimv1.LogicalDeviceLayout {
	return dcimv1.LogicalDeviceLayout_builder{
		DeviceId:  row.LogicalDeviceID.String(),
		PositionX: numericToFloat64(row.PositionX),
		PositionY: numericToFloat64(row.PositionY),
		Updated:   timestamppb.New(row.Updated.Time),
	}.Build()
}
