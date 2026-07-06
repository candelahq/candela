package bigquery

import (
	"context"

	"cloud.google.com/go/bigquery"
)

// BQClient abstracts *bigquery.Client for all Store operations.
// This interface covers read-path queries, IngestSpans (streaming insert +
// MERGE DML), Ping, and schema management (ensureSchema, evolveSchema).
//
// Implementations:
//   - bqClientWrapper (production, wraps *bigquery.Client)
//   - mockBQClient (tests)
type BQClient interface {
	Close() error
	Dataset(string) BQDataset
	Query(string) BQQuery
}

// BQDataset abstracts *bigquery.Dataset.
// Used by Ping, IngestSpans, and ensureSchema.
type BQDataset interface {
	Create(ctx context.Context, md *bigquery.DatasetMetadata) error
	Table(string) BQTable
}

// BQTable abstracts *bigquery.Table.
// Used by Ping (Metadata), IngestSpans (Inserter), and schema management
// (Create, Metadata, Update, FullyQualifiedName).
type BQTable interface {
	Create(ctx context.Context, md *bigquery.TableMetadata) error
	Metadata(ctx context.Context) (*bigquery.TableMetadata, error)
	Update(ctx context.Context, md bigquery.TableMetadataToUpdate, etag string) (*bigquery.TableMetadata, error)
	Inserter() BQInserter
	FullyQualifiedName() string
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

func (w *bqDatasetWrapper) Create(ctx context.Context, md *bigquery.DatasetMetadata) error {
	return w.d.Create(ctx, md)
}

func (w *bqDatasetWrapper) Table(id string) BQTable { return &bqTableWrapper{w.d.Table(id)} }

type bqTableWrapper struct{ t *bigquery.Table }

func (w *bqTableWrapper) Create(ctx context.Context, md *bigquery.TableMetadata) error {
	return w.t.Create(ctx, md)
}

func (w *bqTableWrapper) Metadata(ctx context.Context) (*bigquery.TableMetadata, error) {
	return w.t.Metadata(ctx)
}

func (w *bqTableWrapper) Update(ctx context.Context, md bigquery.TableMetadataToUpdate, etag string) (*bigquery.TableMetadata, error) {
	return w.t.Update(ctx, md, etag)
}

func (w *bqTableWrapper) Inserter() BQInserter {
	return &bqInserterWrapper{w.t.Inserter()}
}

func (w *bqTableWrapper) FullyQualifiedName() string {
	return w.t.FullyQualifiedName()
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
