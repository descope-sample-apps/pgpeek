package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/descope/pgpeek/internal/store"
)

func TestSavedQueries_CRUD(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{result: okResult()})

	// Create
	resp := post(t, ts, "/api/queries", `{"name":"q","description":"d","sql":"SELECT 1"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", resp.StatusCode)
	}
	created := decode[store.SavedQuery](t, resp)
	if created.ID == 0 {
		t.Fatal("no id returned")
	}

	// List
	resp = mustGet(t, ts, "/api/queries")
	list := decode[[]store.SavedQuery](t, resp)
	if len(list) != 1 {
		t.Fatalf("list len = %d", len(list))
	}

	// Update
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/queries/"+itoa(created.ID),
		strings.NewReader(`{"name":"q2","sql":"SELECT 2"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete
	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/api/queries/"+itoa(created.ID), nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSavedQueries_Validation(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})

	cases := []struct {
		name, body string
	}{
		{"missing name", `{"sql":"SELECT 1"}`},
		{"missing sql", `{"name":"x"}`},
		{"non-readonly sql", `{"name":"x","sql":"DELETE FROM t"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := post(t, ts, "/api/queries", c.body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}

func TestSavedQueries_NotFoundAndBadID(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/queries/999", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete missing status = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()

	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/api/queries/abc",
		strings.NewReader(`{"name":"x","sql":"SELECT 1"}`))
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad id status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()
}
