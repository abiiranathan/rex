package rex_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abiiranathan/rex"
	"github.com/stretchr/testify/require"
)

type appState struct {
	Name    string
	Counter *int64
}

func TestWithState(t *testing.T) {
	t.Parallel()
	r := rex.NewRouter(rex.WithState(&appState{Name: "myapp"}))
	r.GET("/state", func(c *rex.Context) error {
		app, ok := rex.GetState[*appState](c)
		require.True(t, ok)
		return c.String(app.Name)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/state", nil))
	require.Equal(t, "myapp", w.Body.String())
}

func TestSetStateReplacesState(t *testing.T) {
	t.Parallel()
	r := rex.NewRouter()
	r.SetState(&appState{Name: "first"})
	r.GET("/state", func(c *rex.Context) error { return c.String(c.State().(*appState).Name) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/state", nil))
	require.Equal(t, "first", w.Body.String())
}

func TestGetStateTypeMismatch(t *testing.T) {
	t.Parallel()
	r := rex.NewRouter(rex.WithState("a string"))
	r.GET("/state", func(c *rex.Context) error {
		app, ok := rex.GetState[*appState](c)
		require.False(t, ok)
		require.Nil(t, app)

		s, ok := rex.GetState[string](c)
		require.True(t, ok)
		require.Equal(t, "a string", s)
		return nil
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/state", nil))
	require.Equal(t, http.StatusOK, w.Code)
}

func TestStateWithoutRouter(t *testing.T) {
	t.Parallel()
	c := rex.NewContext(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), nil)
	require.Nil(t, c.State())

	_, ok := rex.GetState[*appState](c)
	require.False(t, ok)
}

func TestStateSharedAcrossRequests(t *testing.T) {
	t.Parallel()
	counter := new(int64)
	r := rex.NewRouter(rex.WithState(counter))
	r.GET("/inc", func(c *rex.Context) error {
		n, _ := rex.GetState[*int64](c)
		*n++
		return c.String("ok")
	})

	for range 5 {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/inc", nil))
		require.Equal(t, http.StatusOK, w.Code)
	}
	require.Equal(t, int64(5), *counter)
}
