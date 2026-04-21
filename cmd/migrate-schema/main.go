package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

func main() {
	projectID := flag.String("project", "", "GCP Project ID (required)")
	databaseID := flag.String("database", "candela", "Firestore Database ID")
	dryRun := flag.Bool("dry-run", true, "If true, logs changes without applying them")
	flag.Parse()

	if *projectID == "" {
		log.Fatal("--project flag is required")
	}

	ctx := context.Background()
	client, err := firestore.NewClientWithDatabase(ctx, *projectID, *databaseID)
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer func() { _ = client.Close() }()

	fmt.Printf("🕯️  Starting migration (Project: %s, Database: %s, DryRun: %v)\n", *projectID, *databaseID, *dryRun)

	// 1. Migrate Users
	if err := migrateCollection(ctx, client.Collection("users"), map[string]string{
		"DisplayName": "display_name",
		"Role":        "role",
		"Status":      "status",
		"CreatedAt":   "created_at",
		"LastSeenAt":  "last_seen_at",
		"RateLimit":   "rate_limit",
	}, *dryRun); err != nil {
		log.Printf("Error migrating users: %v", err)
	}

	// 2. Migrate Budgets and Grants (subcollections)
	userIter := client.Collection("users").Documents(ctx)
	for {
		userSnap, err := userIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("Error iterating users for subcollections: %v", err)
		}

		// Budgets
		if err := migrateCollection(ctx, userSnap.Ref.Collection("budgets"), map[string]string{
			"LimitUSD":    "limit_usd",
			"SpentUSD":    "spent_usd",
			"TokensUsed":  "tokens_used",
			"PeriodType":  "period_type",
			"PeriodKey":   "period_key",
			"PeriodStart": "period_start",
			"PeriodEnd":   "period_end",
		}, *dryRun); err != nil {
			log.Printf("Error migrating budgets for user %s: %v", userSnap.Ref.ID, err)
		}

		// Grants
		if err := migrateCollection(ctx, userSnap.Ref.Collection("grants"), map[string]string{
			"AmountUSD": "amount_usd",
			"SpentUSD":  "spent_usd",
			"Reason":    "reason",
			"GrantedBy": "granted_by",
			"StartsAt":  "starts_at",
			"ExpiresAt": "expires_at",
			"CreatedAt": "created_at",
		}, *dryRun); err != nil {
			log.Printf("Error migrating grants for user %s: %v", userSnap.Ref.ID, err)
		}
	}

	fmt.Println("✅ Migration complete")
}

func migrateCollection(ctx context.Context, col *firestore.CollectionRef, mapping map[string]string, dryRun bool) error {
	iter := col.Documents(ctx)
	count := 0
	for {
		snap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}

		data := snap.Data()
		updates := []firestore.Update{}
		deletedFields := []string{}

		for oldKey, newKey := range mapping {
			if val, ok := data[oldKey]; ok {
				updates = append(updates, firestore.Update{Path: newKey, Value: val})
				deletedFields = append(deletedFields, oldKey)
			}
		}

		if len(updates) > 0 {
			// Append delete operations so the entire rename is a single atomic Update.
			for _, k := range deletedFields {
				updates = append(updates, firestore.Update{Path: k, Value: firestore.Delete})
			}

			count++
			if dryRun {
				fmt.Printf("[DRY-RUN] Would update %s: renaming %v\n", snap.Ref.Path, deletedFields)
			} else {
				_, err := snap.Ref.Update(ctx, updates)
				if err != nil {
					return fmt.Errorf("failed to update %s: %w", snap.Ref.ID, err)
				}
				fmt.Printf("Updated %s: renamed %d fields\n", snap.Ref.Path, len(deletedFields))
			}
		}
	}
	if count > 0 {
		fmt.Printf("Finished collection %s (updated %d docs)\n", col.Path, count)
	}
	return nil
}
