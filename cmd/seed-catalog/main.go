// Command seed-catalog populates a Firestore model catalog collection with
// model pricing entries. By default it uses the built-in pricing from the
// Calculator; use --from to load entries from an external YAML/JSON file.
//
// Usage:
//
//	go run ./cmd/seed-catalog/ \
//	  --project-id=your-gcp-project \
//	  [--collection=model_catalog] \
//	  [--from=pricing.yaml] \
//	  [--merge] \
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

// seedResult holds the outcome counters from a seedEntries run.
type seedResult struct {
	seeded  int
	skipped int
	errors  int
}

func main() {
	projectID := flag.String("project-id", "", "GCP Project ID (required)")
	databaseID := flag.String("database-id", "(default)", "Firestore database ID")
	collection := flag.String("collection", "model_catalog", "Firestore collection name")
	fromFile := flag.String("from", "", "Read model entries from an external YAML/JSON file instead of embedded pricing")
	merge := flag.Bool("merge", false, "Skip existing entries instead of overwriting (upsert only new)")
	dryRun := flag.Bool("dry-run", false, "Print entries without writing to Firestore")
	flag.Parse()

	if *projectID == "" {
		log.Fatal("--project-id flag is required")
	}

	var (
		entries []catalog.Entry
		source  string
	)

	if *fromFile != "" {
		// Load entries from external file.
		var err error
		entries, err = loadCatalogFile(*fromFile)
		if err != nil {
			log.Fatalf("loading catalog file: %v", err)
		}
		source = *fromFile
	} else {
		// Build the default pricing table from the Calculator.
		entries = buildEntriesFromDefaults()
		source = "embedded defaults"
	}

	fmt.Printf("🕯️  seed-catalog (project=%s database=%s collection=%s dry-run=%v)\n\n",
		*projectID, *databaseID, *collection, *dryRun)
	fmt.Printf("Found %d model pricing entries from %s.\n\n", len(entries), source)

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

	result, err := seedEntries(ctx, store, entries, *merge)
	if err != nil {
		log.Fatalf("seeding: %v", err)
	}

	fmt.Printf("\n✅ Done — %d entries seeded", result.seeded)
	if result.skipped > 0 {
		fmt.Printf(", %d skipped (already exist)", result.skipped)
	}
	if result.errors > 0 {
		fmt.Printf(", %d errors", result.errors)
	}
	fmt.Println(".")
}

// seedEntries writes catalog entries to the store. When merge is true, existing
// entries are skipped (insert-only). All existing IDs are fetched in a single
// List call to avoid N+1 per-entry lookups.
func seedEntries(ctx context.Context, store catalog.ModelCatalogStore, entries []catalog.Entry, merge bool) (seedResult, error) {
	var existing map[string]struct{}

	// In merge mode, batch-fetch all existing entries upfront to build a
	// lookup set. This replaces the previous N+1 store.Get() per entry.
	if merge {
		all, err := store.List(ctx, true) // include disabled
		if err != nil {
			return seedResult{}, fmt.Errorf("listing existing entries for merge: %w", err)
		}
		existing = make(map[string]struct{}, len(all))
		for _, e := range all {
			key := e.Provider + "/" + e.ModelID
			existing[key] = struct{}{}
		}
		slog.Info("merge mode: loaded existing entries", "count", len(existing))
	}

	var result seedResult
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			slog.Error("seeding cancelled", "error", err)
			break
		}

		// In merge mode, skip entries that already exist in the store.
		if merge {
			key := e.Provider + "/" + e.ModelID
			if _, ok := existing[key]; ok {
				fmt.Printf("  [SKIP] %s/%s — already exists\n", e.Provider, e.ModelID)
				result.skipped++
				continue
			}
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
			result.errors++
			continue
		}
		result.seeded++
	}

	return result, nil
}

// buildEntriesFromDefaults creates catalog entries from the compiled-in
// pricing table, applying Anthropic-specific transformations.
func buildEntriesFromDefaults() []catalog.Entry {
	calc := costcalc.New()
	defaults := calc.Defaults() // Already sorted by provider/model.

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
	return entries
}

// vertexModelID converts an Anthropic model name to its Vertex AI equivalent.
// Vertex AI uses dashes where Anthropic uses dots in version numbers.
// e.g. "claude-opus-4.7" → "claude-opus-4-7"
//
// If no dots are present, returns the input unchanged.
func vertexModelID(model string) string {
	return strings.ReplaceAll(model, ".", "-")
}
