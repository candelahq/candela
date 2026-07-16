package bigquery

import (
	"context"
	"sync"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

// ── Mock BQ Client ───────────────────────────────────────────────────
// Captures SQL queries and parameters for assertion in tests.
// Follows the mockTokenVerifier pattern from #513: configurable per-test
// via function fields.

// mockBQClient implements BQClient for testing.
// It queues up mockBQQuery instances to return from Query() calls,
// supporting multi-query methods like QueryTraces which calls Query() twice.
type mockBQClient struct {
	mu      sync.Mutex
	queries []*mockBQQuery // queue of queries to return, consumed FIFO
	dataset *mockBQDataset
	closed  bool
}

func newMockBQClient() *mockBQClient {
	return &mockBQClient{
		dataset: &mockBQDataset{tables: make(map[string]*mockBQTable)},
	}
}

func (m *mockBQClient) Close() error {
	m.closed = true
	return nil
}

func (m *mockBQClient) Dataset(_ string) BQDataset {
	return m.dataset
}

func (m *mockBQClient) Query(sql string) BQQuery {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.queries) == 0 {
		// Return a default mock that captures the SQL but has no rows.
		q := &mockBQQuery{sql: sql}
		return q
	}

	q := m.queries[0]
	m.queries = m.queries[1:]
	q.sql = sql
	return q
}

// enqueueQuery adds a pre-configured mock query to the FIFO queue.
func (m *mockBQClient) enqueueQuery(q *mockBQQuery) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queries = append(m.queries, q)
}

// ── Mock Dataset / Table ─────────────────────────────────────────────

type mockBQDataset struct {
	tables       map[string]*mockBQTable
	createCalled bool
	createErr    error
}

func (d *mockBQDataset) Create(_ context.Context, _ *bigquery.DatasetMetadata) error {
	d.createCalled = true
	if d.createErr != nil {
		return d.createErr
	}
	return nil
}

func (d *mockBQDataset) Table(id string) BQTable {
	if t, ok := d.tables[id]; ok {
		return t
	}
	t := &mockBQTable{id: id}
	d.tables[id] = t
	return t
}

type mockBQTable struct {
	id             string
	createCalled   bool
	createErr      error
	metadataCalled bool
	metadataErr    error
	metadataResult *bigquery.TableMetadata
	updateCalled   bool
	updateErr      error
	updateSchema   bigquery.Schema // captures the schema passed to Update
	inserter       *mockBQInserter
}

func (t *mockBQTable) Create(_ context.Context, _ *bigquery.TableMetadata) error {
	t.createCalled = true
	if t.createErr != nil {
		return t.createErr
	}
	return nil
}

func (t *mockBQTable) Metadata(_ context.Context) (*bigquery.TableMetadata, error) {
	t.metadataCalled = true
	if t.metadataErr != nil {
		return nil, t.metadataErr
	}
	if t.metadataResult != nil {
		return t.metadataResult, nil
	}
	return &bigquery.TableMetadata{}, nil
}

func (t *mockBQTable) Update(_ context.Context, md bigquery.TableMetadataToUpdate, _ string) (*bigquery.TableMetadata, error) {
	t.updateCalled = true
	t.updateSchema = md.Schema
	if t.updateErr != nil {
		return nil, t.updateErr
	}
	return &bigquery.TableMetadata{Schema: md.Schema}, nil
}

func (t *mockBQTable) Inserter() BQInserter {
	if t.inserter != nil {
		return t.inserter
	}
	return &mockBQInserter{}
}

func (t *mockBQTable) FullyQualifiedName() string {
	return "test-project.test_dataset." + t.id
}

// ── Mock Inserter ────────────────────────────────────────────────────

type mockBQInserter struct {
	putCalled bool
	putSrc    interface{}
	putErr    error
}

func (i *mockBQInserter) Put(_ context.Context, src interface{}) error {
	i.putCalled = true
	i.putSrc = src
	if i.putErr != nil {
		return i.putErr
	}
	return nil
}

// ── Mock Query ───────────────────────────────────────────────────────

type mockBQQuery struct {
	sql     string
	params  []bigquery.QueryParameter
	iter    *mockBQRowIterator
	readErr error
	job     *mockBQJob
	runErr  error
}

func (q *mockBQQuery) SetParameters(params []bigquery.QueryParameter) {
	q.params = params
}

func (q *mockBQQuery) Read(_ context.Context) (BQRowIterator, error) {
	if q.readErr != nil {
		return nil, q.readErr
	}
	if q.iter == nil {
		return &mockBQRowIterator{}, nil
	}
	return q.iter, nil
}

func (q *mockBQQuery) Run(_ context.Context) (BQJob, error) {
	if q.runErr != nil {
		return nil, q.runErr
	}
	if q.job == nil {
		return &mockBQJob{}, nil
	}
	return q.job, nil
}

// ── Mock Job ─────────────────────────────────────────────────────────
// NOTE: JobStatus.Err() reads a private field that cannot be set externally,
// so we can only test the waitErr path (job.Wait returning an error).
// The status.Err() branch in IngestSpans is not exercisable via mock.

type mockBQJob struct {
	waitErr error
}

func (j *mockBQJob) Wait(_ context.Context) (*bigquery.JobStatus, error) {
	if j.waitErr != nil {
		return nil, j.waitErr
	}
	return &bigquery.JobStatus{}, nil
}

// ── Mock Row Iterator ────────────────────────────────────────────────
// Uses a nextFunc field so each test can configure exactly what rows
// to return, following the mockTokenVerifier pattern.

type mockBQRowIterator struct {
	nextFunc func(dst interface{}) error
	callIdx  int
}

func (it *mockBQRowIterator) Next(dst interface{}) error {
	if it.nextFunc == nil {
		return iterator.Done
	}
	it.callIdx++
	return it.nextFunc(dst)
}
