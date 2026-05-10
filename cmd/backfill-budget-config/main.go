// Command backfill-budget-config populates the stable budgets/config sentinel
// document for all existing users that have a daily budget but no config doc.
//
// This is a one-time migration required after deploying the budget auto-rollover
// feature (#9). Without this, existing users have no budgets/config doc and
// DeductSpend falls back to the grant-only path, leaving them unmetered after
// midnight until their next SetBudget call.
//
// Usage:
//
//	go run ./cmd/backfill-budget-config \
//	  --project=your-gcp-project \
//	  --database=candela \
//	  [--dry-run=false]
//
// The command is safe to run multiple times — it skips users that already
// have a config doc (Set with MergeSpecific is idempotent).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sort"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	usersCol   = "users"
	budgetsCol = "budgets"
	configDoc  = "config" // budgets/config sentinel doc ID
)

func main() {
	projectID := flag.String("project", "", "GCP Project ID (required)")
	databaseID := flag.String("database", "candela", "Firestore Database ID")
	dryRun := flag.Bool("dry-run", true, "If true, logs what would change without applying")
	flag.Parse()

	if *projectID == "" {
		log.Fatal("--project flag is required")
	}

	ctx := context.Background()
	client, err := firestore.NewClientWithDatabase(ctx, *projectID, *databaseID)
	if err != nil {
		log.Fatalf("firestore client: %v", err)
	}
	defer func() { _ = client.Close() }()

	fmt.Printf("🕯️  Backfill budgets/config (project=%s db=%s dry-run=%v)\n\n",
		*projectID, *databaseID, *dryRun)

	var (
		total    int
		skipped  int
		migrated int
		errCount int
	)

	userIter := client.Collection(usersCol).Documents(ctx)
	defer userIter.Stop()
	for {
		userSnap, err := userIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("iterating users: %v", err)
		}
		total++
		userID := userSnap.Ref.ID

		// Check if config doc already exists — idempotency.
		configRef := userSnap.Ref.Collection(budgetsCol).Doc(configDoc)
		configSnap, err := configRef.Get(ctx)
		if err != nil && status.Code(err) != codes.NotFound {
			log.Printf("  [ERR] %s: reading config doc: %v", userID, err)
			errCount++
			continue
		}
		if configSnap != nil && configSnap.Exists() {
			skipped++
			fmt.Printf("  [SKIP] %s — config doc already exists\n", userID)
			continue
		}

		// Find the most recent budget doc by period key (YYYY-MM-DD, sorts lexicographically).
		budgetIter := userSnap.Ref.Collection(budgetsCol).
			Where("limit_usd", ">", 0).
			Documents(ctx)
		var budgetDocs []*firestore.DocumentSnapshot
		for {
			snap, err := budgetIter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				log.Printf("  [ERR] %s: listing budgets: %v", userID, err)
				break
			}
			// Skip the config doc itself (shouldn't exist but be safe).
			if snap.Ref.ID == configDoc {
				continue
			}
			budgetDocs = append(budgetDocs, snap)
		}
		budgetIter.Stop()

		if len(budgetDocs) == 0 {
			fmt.Printf("  [SKIP] %s — no budget docs found\n", userID)
			skipped++
			continue
		}

		// Sort descending by period key → latest period is [0].
		sort.Slice(budgetDocs, func(i, j int) bool {
			return budgetDocs[i].Ref.ID > budgetDocs[j].Ref.ID
		})
		latest := budgetDocs[0].Data()

		limitUSD := 0.0
		switch v := latest["limit_usd"].(type) {
		case float64:
			limitUSD = v
		case int64:
			limitUSD = float64(v)
		case int32:
			limitUSD = float64(v)
		}
		periodType, _ := latest["period_type"].(string)
		if periodType == "" {
			periodType = "daily"
		}

		fmt.Printf("  [MIGRATE] %s — limit_usd=%.2f period_type=%s (from period %s)\n",
			userID, limitUSD, periodType, budgetDocs[0].Ref.ID)

		if !*dryRun {
			_, err := configRef.Set(ctx, map[string]interface{}{
				"limit_usd":   limitUSD,
				"period_type": periodType,
				"user_id":     userID,
			}, firestore.MergeAll)
			if err != nil {
				log.Printf("  [ERR] %s: writing config doc: %v", userID, err)
				errCount++
				continue
			}
		}
		migrated++
	}

	fmt.Printf("\n✅ Done — %d users total, %d migrated, %d skipped (already done), %d errors\n",
		total, migrated, skipped, errCount)
	if *dryRun && migrated > 0 {
		fmt.Println("   Re-run with --dry-run=false to apply.")
	}
}
