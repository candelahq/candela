// Command seed-catalog populates a Firestore model catalog collection with the
// built-in default pricing from the Calculator.
//
// Usage:
//
//	go run ./cmd/seed-catalog/ \
//	  --project-id=your-gcp-project \
//	  [--collection=model_catalog] \
//	  [--dry-run]
//
// In dry-run mode the tool prints what would be written without touching
// Firestore. This is the default for safety.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"strings"

	"cloud.google.com/go/firestore"

	"github.com/candelahq/candela/pkg/catalog"
	"github.com/candelahq/candela/pkg/costcalc"
)

func main() {
	projectID := flag.String("project-id", "", "GCP Project ID (required)")
	databaseID := flag.String("database-id", "(default)", "Firestore database ID")
	collection := flag.String("collection", "model_catalog", "Firestore collection name")
	dryRun := flag.Bool("dry-run", false, "Print entries without writing to Firestore")
	flag.Parse()

	if *projectID == "" {
		log.Fatal("--project-id flag is required")
	}

	// Build the default pricing table from the Calculator.
	calc := costcalc.New()
	defaults := calc.Defaults() // Already sorted by provider/model.

	// Convert ModelPricing → catalog.Entry.
	entries := make([]catalog.Entry, 0, len(defaults))
	for _, p := range defaults {
		e := catalog.Entry{
			ModelID:              p.Model,
			Provider:             p.Provider,
			InputPerMillion:      p.InputPerMillion,
			OutputPerMillion:     p.OutputPerMillion,
			InputPerMillionHigh:  p.InputPerMillionHigh,
			OutputPerMillionHigh: p.OutputPerMillionHigh,
			TierThresholdTokens:  p.TierThresholdTokens,
			DiscountPercent:      p.DiscountPercent,
			Enabled:              true,
		}

		// For Anthropic models routed through Vertex AI, set the
		// provider-specific model ID (Vertex uses dashes, not dots)
		// and default to the "global" region endpoint.
		if p.Provider == "anthropic" {
			if vid := vertexModelID(p.Model); vid != p.Model {
				e.ProviderModelID = vid
			}
			e.Region = "global"
		}

		entries = append(entries, e)
	}

	fmt.Printf("🕯️  seed-catalog (project=%s database=%s collection=%s dry-run=%v)\n\n",
		*projectID, *databaseID, *collection, *dryRun)
	fmt.Printf("Found %d default model pricing entries.\n\n", len(entries))

	if *dryRun {
		for _, e := range entries {
			fmt.Printf("  [DRY-RUN] %s/%s — in=$%.4f/M out=$%.4f/M",
				e.Provider, e.ModelID, e.InputPerMillion, e.OutputPerMillion)
			if e.TierThresholdTokens > 0 {
				fmt.Printf(" (high: in=$%.4f out=$%.4f >%dK)",
					e.InputPerMillionHigh, e.OutputPerMillionHigh, e.TierThresholdTokens/1000)
			}
			fmt.Println()
		}
		fmt.Printf("\n✅ Dry run complete — %d entries would be seeded.\n", len(entries))
		fmt.Println("   Re-run without --dry-run to apply.")
		return
	}

	// Create Firestore client and store.
	ctx := context.Background()
	client, err := firestore.NewClientWithDatabase(ctx, *projectID, *databaseID)
	if err != nil {
		log.Fatalf("firestore client: %v", err)
	}
	defer func() { _ = client.Close() }()

	store := catalog.NewFirestoreStore(client, *collection)

	var (
		seeded   int
		errCount int
	)
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			slog.Error("seeding cancelled", "error", err)
			break
		}
		fmt.Printf("  [SEED] %s/%s — in=$%.4f/M out=$%.4f/M",
			e.Provider, e.ModelID, e.InputPerMillion, e.OutputPerMillion)
		if e.TierThresholdTokens > 0 {
			fmt.Printf(" (high: in=$%.4f out=$%.4f >%dK)",
				e.InputPerMillionHigh, e.OutputPerMillionHigh, e.TierThresholdTokens/1000)
		}
		fmt.Println()

		if err := store.Update(ctx, e); err != nil {
			fmt.Printf("  [ERR]  %s/%s: %v\n", e.Provider, e.ModelID, err)
			errCount++
			continue
		}
		seeded++
	}

	fmt.Printf("\n✅ Done — %d entries seeded, %d errors.\n", seeded, errCount)
}

// vertexModelID converts an Anthropic model name to its Vertex AI equivalent.
// Vertex AI uses dashes where Anthropic uses dots in version numbers.
// e.g. "claude-opus-4.7" → "claude-opus-4-7"
//
// If no dots are present, returns the input unchanged.
func vertexModelID(model string) string {
	return strings.ReplaceAll(model, ".", "-")
}
