package server

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/descope-sample-apps/pgpeek/internal/db"
)

func TestTableData_ParsesEveryFilterOperatorAndSearch(t *testing.T) {
	q := &fakeQuerier{result: okResult()}
	ts, _ := newTestServer(t, q)
	params := url.Values{
		"search": []string{"acme"},
		"f": []string{
			"id:eq:1",
			"id:ne:2",
			"id:lt:3",
			"id:lte:4",
			"id:gt:5",
			"id:gte:6",
			"name:like:a:b",
			"name:ilike:%A%",
			"deleted_at:is_null",
			"deleted_at:is_not_null",
		},
	}
	resp := mustGet(t, ts, "/api/tables/public/users/data?"+params.Encode())
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if q.lastQuery.Search != "acme" {
		t.Fatalf("search = %q", q.lastQuery.Search)
	}
	want := []db.Filter{
		{Column: "id", Op: "eq", Value: "1"},
		{Column: "id", Op: "ne", Value: "2"},
		{Column: "id", Op: "lt", Value: "3"},
		{Column: "id", Op: "lte", Value: "4"},
		{Column: "id", Op: "gt", Value: "5"},
		{Column: "id", Op: "gte", Value: "6"},
		{Column: "name", Op: "like", Value: "a:b"},
		{Column: "name", Op: "ilike", Value: "%A%"},
		{Column: "deleted_at", Op: "is_null"},
		{Column: "deleted_at", Op: "is_not_null"},
	}
	if len(q.lastQuery.Filters) != len(want) {
		t.Fatalf("filters = %+v", q.lastQuery.Filters)
	}
	for i := range want {
		if q.lastQuery.Filters[i] != want[i] {
			t.Errorf("filter %d = %+v, want %+v", i, q.lastQuery.Filters[i], want[i])
		}
	}
}
