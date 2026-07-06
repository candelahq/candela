package bigquery

import (
	"context"

	"cloud.google.com/go/bigquery"
)

// BQClient abstracts *bigquery.Client for the read path.
// This interface covers only the methods needed by read-path queries
// (QueryTraces, SearchSpans, GetUsageSummary, etc.) and Ping.
//
// Implementations:
//   - bqClientWrapper (production, wraps *bigquery.Client)
//   - mockBQClient (tests)
type BQClient interface {
	Close() error
	Dataset(string) BQDataset
	Query(string) BQQuery
}

// BQDataset abstracts *bigquery.Dataset (used by Ping).
type BQDataset interface {
	Table(string) BQTable
}

// BQTable abstracts *bigquery.Table (Ping reads table metadata).
type BQTable interface {
	Metadata(ctx context.Context) (*bigquery.TableMetadata, error)
}

// BQQuery abstracts *bigquery.Query for parameterized read queries.
// SetParameters wraps the field assignment q.Parameters = [...] as a method
// call so the interface stays clean.
type BQQuery interface {
	SetParameters([]bigquery.QueryParameter)
	Read(ctx context.Context) (BQRowIterator, error)
}

// BQRowIterator abstracts *bigquery.RowIterator for scanning query results.
type BQRowIterator interface {
	Next(dst interface{}) error
}

// ── Production wrappers ──────────────────────────────────────────────
// These delegate directly to the real BigQuery SDK types.

type bqClientWrapper struct{ c *bigquery.Client }

func (w *bqClientWrapper) Close() error                { return w.c.Close() }
func (w *bqClientWrapper) Dataset(id string) BQDataset { return &bqDatasetWrapper{w.c.Dataset(id)} }
func (w *bqClientWrapper) Query(q string) BQQuery      { return &bqQueryWrapper{w.c.Query(q)} }

type bqDatasetWrapper struct{ d *bigquery.Dataset }

func (w *bqDatasetWrapper) Table(id string) BQTable { return &bqTableWrapper{w.d.Table(id)} }

type bqTableWrapper struct{ t *bigquery.Table }

func (w *bqTableWrapper) Metadata(ctx context.Context) (*bigquery.TableMetadata, error) {
	return w.t.Metadata(ctx)
}

type bqQueryWrapper struct{ q *bigquery.Query }

func (w *bqQueryWrapper) SetParameters(params []bigquery.QueryParameter) {
	w.q.Parameters = params
}

func (w *bqQueryWrapper) Read(ctx context.Context) (BQRowIterator, error) {
	it, err := w.q.Read(ctx)
	if err != nil {
		return nil, err
	}
	return &bqRowIteratorWrapper{it}, nil
}

type bqRowIteratorWrapper struct{ it *bigquery.RowIterator }

func (w *bqRowIteratorWrapper) Next(dst interface{}) error { return w.it.Next(dst) }
