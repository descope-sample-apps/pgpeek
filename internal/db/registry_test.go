package db

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestNewRegistry_lists_metadata_without_dsn_when_entries_have_private_names(t *testing.T) {
	// Given: registry entries carry public ids/names and pool handles only.
	registry, err := NewRegistry([]RegistryEntry{
		{ID: "billing", Name: "Billing", Pool: &Pool{pool: &fakePool{}, rowCap: 100}, Default: true},
		{ID: "support", Name: "Support", Pool: &Pool{pool: &fakePool{}, rowCap: 200}},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	// When: callers request API-safe metadata.
	got := registry.List()

	// Then: only id/name are exposed, in entry order.
	want := []PoolMetadata{{ID: "billing", Name: "Billing"}, {ID: "support", Name: "Support"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %#v, want %#v", got, want)
	}
}

func TestRegistry_pool_returns_default_when_selection_empty(t *testing.T) {
	// Given: registry has explicit default and another pool.
	defaultPool := &Pool{pool: &fakePool{}, rowCap: 100}
	otherPool := &Pool{pool: &fakePool{}, rowCap: 200}
	registry, err := NewRegistry([]RegistryEntry{
		{ID: "billing", Name: "Billing", Pool: defaultPool, Default: true},
		{ID: "support", Name: "Support", Pool: otherPool},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	// When: caller selects empty id.
	got, err := registry.Pool("")

	// Then: default pool is returned, not an unknown-id error.
	if err != nil {
		t.Fatalf("Pool(empty): %v", err)
	}
	if got != defaultPool {
		t.Fatalf("Pool(empty) returned %p, want default %p", got, defaultPool)
	}
	if registry.DefaultID() != "billing" {
		t.Fatalf("DefaultID() = %q, want billing", registry.DefaultID())
	}
}

func TestRegistry_pool_returns_requested_pool_when_id_known(t *testing.T) {
	// Given: registry has two named pools.
	defaultPool := &Pool{pool: &fakePool{}, rowCap: 100}
	otherPool := &Pool{pool: &fakePool{}, rowCap: 200}
	registry, err := NewRegistry([]RegistryEntry{
		{ID: "billing", Name: "Billing", Pool: defaultPool},
		{ID: "support", Name: "Support", Pool: otherPool},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	// When: caller selects a known id.
	got, err := registry.Pool("support")

	// Then: matching pool is returned.
	if err != nil {
		t.Fatalf("Pool(support): %v", err)
	}
	if got != otherPool {
		t.Fatalf("Pool(support) returned %p, want %p", got, otherPool)
	}
}

func TestRegistry_pool_returns_not_found_when_non_empty_id_unknown(t *testing.T) {
	// Given: registry has one pool.
	registry, err := NewRegistry([]RegistryEntry{{ID: "billing", Name: "Billing", Pool: &Pool{pool: &fakePool{}, rowCap: 100}}})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	// When: caller selects an unknown non-empty id.
	_, err = registry.Pool("missing")

	// Then: caller can distinguish unknown id from default selection.
	if !errors.Is(err, ErrPoolNotFound) {
		t.Fatalf("Pool(missing) error = %v, want ErrPoolNotFound", err)
	}
}

func TestRegistry_ping_visits_pools_in_metadata_order_and_stops_on_error(t *testing.T) {
	// Given: second pool fails ping.
	first := &orderedPool{name: "first"}
	second := &orderedPool{name: "second", pingErr: errors.New("down")}
	third := &orderedPool{name: "third"}
	registry, err := NewRegistry([]RegistryEntry{
		{ID: "first", Name: "First", Pool: &Pool{pool: first, rowCap: 10}},
		{ID: "second", Name: "Second", Pool: &Pool{pool: second, rowCap: 10}},
		{ID: "third", Name: "Third", Pool: &Pool{pool: third, rowCap: 10}},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	// When: readiness pings all pools.
	err = registry.Ping(context.Background())

	// Then: order is deterministic and ping stops at failing pool.
	if err == nil {
		t.Fatal("Ping() expected error")
	}
	got := append([]string{}, first.pings...)
	got = append(got, second.pings...)
	if want := []string{"first", "second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ping order = %v, want %v", got, want)
	}
	if len(third.pings) != 0 {
		t.Fatalf("third pool pinged after error: %v", third.pings)
	}
}

func TestRegistry_ping_returns_nil_when_all_pools_healthy(t *testing.T) {
	// Given: every registered pool can be pinged.
	first := &orderedPool{name: "first"}
	second := &orderedPool{name: "second"}
	registry, err := NewRegistry([]RegistryEntry{
		{ID: "first", Name: "First", Pool: &Pool{pool: first, rowCap: 10}},
		{ID: "second", Name: "Second", Pool: &Pool{pool: second, rowCap: 10}},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	// When: readiness pings the registry.
	err = registry.Ping(context.Background())

	// Then: all pools are checked and no error is returned.
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	got := append([]string{}, first.pings...)
	got = append(got, second.pings...)
	if want := []string{"first", "second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ping order = %v, want %v", got, want)
	}
}

func TestRegistry_close_visits_all_pools_in_reverse_metadata_order(t *testing.T) {
	// Given: registry has three pools.
	closeOrder := make([]string, 0, 3)
	registry, err := NewRegistry([]RegistryEntry{
		{ID: "first", Name: "First", Pool: &Pool{pool: &orderedPool{name: "first", closes: &closeOrder}, rowCap: 10}},
		{ID: "second", Name: "Second", Pool: &Pool{pool: &orderedPool{name: "second", closes: &closeOrder}, rowCap: 10}},
		{ID: "third", Name: "Third", Pool: &Pool{pool: &orderedPool{name: "third", closes: &closeOrder}, rowCap: 10}},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	// When: registry closes.
	registry.Close()

	// Then: every pool closes in deterministic reverse order.
	want := []string{"third", "second", "first"}
	if !reflect.DeepEqual(closeOrder, want) {
		t.Fatalf("close order = %v, want %v", closeOrder, want)
	}
}

func TestNewRegistry_rejects_invalid_entries(t *testing.T) {
	tests := []struct {
		name    string
		entries []RegistryEntry
	}{
		{name: "empty registry", entries: nil},
		{name: "empty id", entries: []RegistryEntry{{ID: "", Name: "Billing", Pool: &Pool{pool: &fakePool{}}}}},
		{name: "empty name", entries: []RegistryEntry{{ID: "billing", Name: "", Pool: &Pool{pool: &fakePool{}}}}},
		{name: "nil pool", entries: []RegistryEntry{{ID: "billing", Name: "Billing", Pool: nil}}},
		{name: "duplicate id", entries: []RegistryEntry{{ID: "billing", Name: "Billing", Pool: &Pool{pool: &fakePool{}}}, {ID: "billing", Name: "Other", Pool: &Pool{pool: &fakePool{}}}}},
		{name: "multiple defaults", entries: []RegistryEntry{{ID: "billing", Name: "Billing", Pool: &Pool{pool: &fakePool{}}, Default: true}, {ID: "support", Name: "Support", Pool: &Pool{pool: &fakePool{}}, Default: true}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given: invalid registry input.

			// When: registry is constructed.
			_, err := NewRegistry(tc.entries)

			// Then: construction fails before runtime lookup.
			if err == nil {
				t.Fatal("NewRegistry expected error")
			}
		})
	}
}

type orderedPool struct {
	fakePool
	name    string
	pingErr error
	pings   []string
	closes  *[]string
}

func (p *orderedPool) Ping(context.Context) error {
	p.pings = append(p.pings, p.name)
	return p.pingErr
}

func (p *orderedPool) Close() {
	if p.closes != nil {
		*p.closes = append(*p.closes, p.name)
	}
}
