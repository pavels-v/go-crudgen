package restapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"example.com/blogservice/internal/blog/domain"
)

type fakeRepository[K comparable, M, P any] struct {
	mu      sync.Mutex
	items   map[K]M
	key     func(*M) *K
	nextKey func() K
}

func newFakeRepository[K comparable, M, P any](key func(*M) *K, nextKey func() K) *fakeRepository[K, M, P] {
	return &fakeRepository[K, M, P]{items: make(map[K]M), key: key, nextKey: nextKey}
}

func (r *fakeRepository[K, M, P]) Create(_ context.Context, m *M) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.nextKey != nil {
		*r.key(m) = r.nextKey()
	}

	k := *r.key(m)
	if _, ok := r.items[k]; ok {
		return domain.ErrAlreadyExists
	}

	r.items[k] = *m

	return nil
}

func (r *fakeRepository[K, M, P]) Get(_ context.Context, id K) (*M, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.items[id]
	if !ok {
		return nil, domain.ErrNotFound
	}

	return &m, nil
}

func (r *fakeRepository[K, M, P]) List(_ context.Context, _ P) ([]M, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]M, 0, len(r.items))
	for _, m := range r.items {
		out = append(out, m)
	}

	return out, nil
}

func (r *fakeRepository[K, M, P]) Update(_ context.Context, m *M) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	k := *r.key(m)
	if _, ok := r.items[k]; !ok {
		return domain.ErrNotFound
	}

	r.items[k] = *m

	return nil
}

func (r *fakeRepository[K, M, P]) Delete(_ context.Context, id K) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[id]; !ok {
		return domain.ErrNotFound
	}

	delete(r.items, id)

	return nil
}

func serve(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body)))

	return rec
}
