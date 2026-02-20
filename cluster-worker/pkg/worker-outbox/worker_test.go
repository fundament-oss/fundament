package worker_outbox

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
)

func TestEntityFromRow_Cluster(t *testing.T) {
	clusterID := uuid.New()
	row := &db.OutboxGetAndLockRow{
		ClusterID: pgtype.UUID{Bytes: clusterID, Valid: true},
	}

	entityType, entityID := entityFromRow(row)

	if entityType != handler.EntityCluster {
		t.Errorf("expected EntityCluster, got %q", entityType)
	}
	if entityID != clusterID {
		t.Errorf("expected %s, got %s", clusterID, entityID)
	}
}

func TestEntityFromRow_Namespace(t *testing.T) {
	nsID := uuid.New()
	row := &db.OutboxGetAndLockRow{
		NamespaceID: pgtype.UUID{Bytes: nsID, Valid: true},
	}

	entityType, entityID := entityFromRow(row)

	if entityType != handler.EntityNamespace {
		t.Errorf("expected EntityNamespace, got %q", entityType)
	}
	if entityID != nsID {
		t.Errorf("expected %s, got %s", nsID, entityID)
	}
}

func TestEntityFromRow_ProjectMember(t *testing.T) {
	pmID := uuid.New()
	row := &db.OutboxGetAndLockRow{
		ProjectMemberID: pgtype.UUID{Bytes: pmID, Valid: true},
	}

	entityType, entityID := entityFromRow(row)

	if entityType != handler.EntityProjectMember {
		t.Errorf("expected EntityProjectMember, got %q", entityType)
	}
	if entityID != pmID {
		t.Errorf("expected %s, got %s", pmID, entityID)
	}
}

func TestEntityFromRow_Project(t *testing.T) {
	projID := uuid.New()
	row := &db.OutboxGetAndLockRow{
		ProjectID: pgtype.UUID{Bytes: projID, Valid: true},
	}

	entityType, entityID := entityFromRow(row)

	if entityType != handler.EntityProject {
		t.Errorf("expected EntityProject, got %q", entityType)
	}
	if entityID != projID {
		t.Errorf("expected %s, got %s", projID, entityID)
	}
}

func TestEntityFromRow_NoFKPanics(t *testing.T) {
	row := &db.OutboxGetAndLockRow{
		ID: uuid.New(),
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when no FK column is set")
		}
	}()

	entityFromRow(row)
}

func TestEntityFromRow_Priority(t *testing.T) {
	// When multiple FKs are set (shouldn't happen, but test precedence), cluster wins.
	clusterID := uuid.New()
	nsID := uuid.New()
	row := &db.OutboxGetAndLockRow{
		ClusterID:   pgtype.UUID{Bytes: clusterID, Valid: true},
		NamespaceID: pgtype.UUID{Bytes: nsID, Valid: true},
	}

	entityType, entityID := entityFromRow(row)

	if entityType != handler.EntityCluster {
		t.Errorf("expected EntityCluster (highest priority), got %q", entityType)
	}
	if entityID != clusterID {
		t.Errorf("expected cluster ID %s, got %s", clusterID, entityID)
	}
}

func TestDurationToInterval(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		wantUs   int64
	}{
		{"500ms", 500 * time.Millisecond, 500_000},
		{"1s", time.Second, 1_000_000},
		{"1m", time.Minute, 60_000_000},
		{"zero", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interval := durationToInterval(tt.duration)

			if !interval.Valid {
				t.Error("expected Valid to be true")
			}
			if interval.Microseconds != tt.wantUs {
				t.Errorf("expected %d microseconds, got %d", tt.wantUs, interval.Microseconds)
			}
		})
	}
}
