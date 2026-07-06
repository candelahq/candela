package bigquery

import (
	"context"
	"sync"

	"cloud.google.com/go/bigquery"
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
	tables map[string]*mockBQTable
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
	metadataCalled bool
	metadataErr    error
}

func (t *mockBQTable) Metadata(_ context.Context) (*bigquery.TableMetadata, error) {
	t.metadataCalled = true
	if t.metadataErr != nil {
		return nil, t.metadataErr
	}
	return &bigquery.TableMetadata{}, nil
}

// ── Mock Query ───────────────────────────────────────────────────────

type mockBQQuery struct {
	sql     string
	params  []bigquery.QueryParameter
	iter    *mockBQRowIterator
	readErr error
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

// ── Mock Row Iterator ────────────────────────────────────────────────
// Uses a nextFunc field so each test can configure exactly what rows
// to return, following the mockTokenVerifier pattern.

type mockBQRowIterator struct {
	nextFunc func(dst interface{}) error
	callIdx  int
}

func (it *mockBQRowIterator) Next(dst interface{}) error {
	if it.nextFunc == nil {
		return iteratorDone()
	}
	it.callIdx++
	return it.nextFunc(dst)
}

// iteratorDone returns the google.golang.org/api/iterator.Done sentinel.
// Avoids importing the package in the test helpers.
func iteratorDone() error {
	return iterDoneErr
}

// iterDoneErr is set by init() in the test file to avoid an import cycle.
// See query_test.go for the initialization.
var iterDoneErr error
