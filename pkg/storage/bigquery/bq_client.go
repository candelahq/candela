package bigquery

import (
	"context"

	"cloud.google.com/go/bigquery"
)

// BQClient abstracts *bigquery.Client for the read and write paths.
// This interface covers methods needed by read-path queries
// (QueryTraces, SearchSpans, GetUsageSummary, etc.), Ping, and
// IngestSpans (streaming insert + MERGE DML).
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

// BQTable abstracts *bigquery.Table.
// Ping reads table metadata; IngestSpans uses the inserter.
type BQTable interface {
	Metadata(ctx context.Context) (*bigquery.TableMetadata, error)
	Inserter() BQInserter
}

// BQInserter abstracts *bigquery.Inserter for streaming inserts.
type BQInserter interface {
	Put(ctx context.Context, src interface{}) error
}

// BQQuery abstracts *bigquery.Query for parameterized queries.
// SetParameters wraps the field assignment q.Parameters = [...] as a method
// call so the interface stays clean.
// Read is used by read-path queries; Run is used by write-path DML.
type BQQuery interface {
	SetParameters([]bigquery.QueryParameter)
	Read(ctx context.Context) (BQRowIterator, error)
	Run(ctx context.Context) (BQJob, error)
}

// BQJob abstracts *bigquery.Job (used by IngestSpans pessimistic MERGE path).
type BQJob interface {
	Wait(ctx context.Context) (*bigquery.JobStatus, error)
}

// BQRowIterator abstracts *bigquery.RowIterator for scanning query results.
type BQRowIterator interface {
	Next(dst interface{}) error
}

// ── Production wrappers ──────────────────────────────────────────────
// These delegate directly to the real BigQuery SDK types.

var _ BQClient = (*bqClientWrapper)(nil)
var _ BQDataset = (*bqDatasetWrapper)(nil)
var _ BQTable = (*bqTableWrapper)(nil)
var _ BQInserter = (*bqInserterWrapper)(nil)
var _ BQQuery = (*bqQueryWrapper)(nil)
var _ BQJob = (*bqJobWrapper)(nil)
var _ BQRowIterator = (*bqRowIteratorWrapper)(nil)

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

func (w *bqTableWrapper) Inserter() BQInserter {
	return &bqInserterWrapper{w.t.Inserter()}
}

type bqInserterWrapper struct{ i *bigquery.Inserter }

func (w *bqInserterWrapper) Put(ctx context.Context, src interface{}) error {
	return w.i.Put(ctx, src)
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

func (w *bqQueryWrapper) Run(ctx context.Context) (BQJob, error) {
	job, err := w.q.Run(ctx)
	if err != nil {
		return nil, err
	}
	return &bqJobWrapper{job}, nil
}

type bqJobWrapper struct{ j *bigquery.Job }

func (w *bqJobWrapper) Wait(ctx context.Context) (*bigquery.JobStatus, error) {
	return w.j.Wait(ctx)
}

type bqRowIteratorWrapper struct{ it *bigquery.RowIterator }

func (w *bqRowIteratorWrapper) Next(dst interface{}) error { return w.it.Next(dst) }
