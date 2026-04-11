package usersync

import (
	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
)

// ReconcileActionType identifies the kind of reconciliation action.
type ReconcileActionType int

const (
	ActionEnsureSA  ReconcileActionType = iota // Create or update a ServiceAccount
	ActionDeleteSA                             // Delete a ServiceAccount by name
	ActionEnsureCRB                            // Create or update a ClusterRoleBinding
	ActionDeleteCRB                            // Delete a ClusterRoleBinding by name
)

// ReconcileAction is a single step in a reconciliation plan.
type ReconcileAction struct {
	Type        ReconcileActionType
	UserID      uuid.UUID // For EnsureSA/EnsureCRB
	Email       string    // For EnsureSA/EnsureCRB (annotation value)
	AccessLevel string    // For EnsureSA/EnsureCRB
	Name        string    // Resource name for delete actions
}

// ReconcilePlan is an ordered list of reconciliation actions for a single cluster.
type ReconcilePlan []ReconcileAction

// buildReconcilePlan compares desired state (from DB) against actual state (from shoot)
// and returns the actions needed to converge them. This is a pure function with no side effects.
func buildReconcilePlan(desired []db.UserListForClusterRow, actualSAs, actualCRBs []shoot.ResourceInfo) ReconcilePlan {
	desiredByUserID := make(map[uuid.UUID]db.UserListForClusterRow, len(desired))
	for _, u := range desired {
		desiredByUserID[u.UserID] = u
	}

	actualSAsByUserID, orphanSANames := groupResourcesByUserID(actualSAs)
	actualCRBsByUserID, orphanCRBNames := groupResourcesByUserID(actualCRBs)

	var plan ReconcilePlan

	for userID, desired := range desiredByUserID {
		email := ""
		if desired.Email.Valid {
			email = desired.Email.String
		}

		labels := map[string]string{
			shoot.LabelUserID: userID.String(),
		}
		annotations := map[string]string{
			shoot.AnnotationUserName: email,
		}

		hasHealthySA, duplicateSANames := classifyServiceAccounts(actualSAsByUserID[userID], userID, labels, annotations)
		hasHealthyCRB, duplicateCRBNames := classifyClusterRoleBindings(actualCRBsByUserID[userID], userID, labels, annotations)

		switch desired.AccessLevel {
		case "admin":
			if !hasHealthySA {
				plan = append(plan, ReconcileAction{Type: ActionEnsureSA, UserID: userID, Email: email, AccessLevel: "admin"})
			}
			if !hasHealthyCRB {
				plan = append(plan, ReconcileAction{Type: ActionEnsureCRB, UserID: userID, Email: email, AccessLevel: "admin"})
			}
			for _, name := range duplicateSANames {
				plan = append(plan, ReconcileAction{Type: ActionDeleteSA, Name: name})
			}
			for _, name := range duplicateCRBNames {
				plan = append(plan, ReconcileAction{Type: ActionDeleteCRB, Name: name})
			}
			delete(orphanCRBNames, shoot.CRBName(userID))

		case "member":
			if !hasHealthySA {
				plan = append(plan, ReconcileAction{Type: ActionEnsureSA, UserID: userID, Email: email, AccessLevel: "member"})
			}
			for _, name := range duplicateSANames {
				plan = append(plan, ReconcileAction{Type: ActionDeleteSA, Name: name})
			}
			// Members should not have CRBs — delete all
			for _, name := range resourceNames(actualCRBsByUserID[userID]) {
				plan = append(plan, ReconcileAction{Type: ActionDeleteCRB, Name: name})
			}
		}

		delete(orphanSANames, shoot.SAName(userID))
		delete(actualSAsByUserID, userID)
		delete(actualCRBsByUserID, userID)
	}

	// Orphaned SAs (remaining grouped + invalid metadata)
	for _, resources := range actualSAsByUserID {
		for _, name := range resourceNames(resources) {
			plan = append(plan, ReconcileAction{Type: ActionDeleteSA, Name: name})
		}
	}
	for _, name := range sortedResourceNames(orphanSANames) {
		plan = append(plan, ReconcileAction{Type: ActionDeleteSA, Name: name})
	}

	// Orphaned CRBs
	for _, resources := range actualCRBsByUserID {
		for _, name := range resourceNames(resources) {
			plan = append(plan, ReconcileAction{Type: ActionDeleteCRB, Name: name})
		}
	}
	for _, name := range sortedResourceNames(orphanCRBNames) {
		plan = append(plan, ReconcileAction{Type: ActionDeleteCRB, Name: name})
	}

	return plan
}
