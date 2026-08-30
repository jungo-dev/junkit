package filter_test

import (
	"reflect"
	"testing"

	"github.com/jungo-dev/junkit/response/filter"
)

type address struct {
	City   string `json:"city"`
	Street string `json:"street"`
}

type user struct {
	ID         int      `json:"id"`
	Name       string   `json:"name"`
	Email      string   `json:"email"`
	Bio        string   `json:"bio,omitempty"`
	LoginCount int      `json:"login_count,omitempty"`
	Verified   bool     `json:"verified,omitempty"`
	Address    address  `json:"address"`
	Tags       []string `json:"tags"`
	private    string   //nolint:unused // exercises the "unexported fields are skipped" case
}

func sampleUser() user {
	return user{
		ID:    1,
		Name:  "Jane",
		Email: "jane@example.com",
		Address: address{
			City:   "Hanoi",
			Street: "Ba Trieu",
		},
		Tags: []string{"admin", "beta"},
	}
}

func TestFilter_NoFieldsOrOmit_ReturnsDataUnchanged(t *testing.T) {
	u := sampleUser()

	got, err := filter.Filter(u, nil, nil)
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}
	if !reflect.DeepEqual(got, u) {
		t.Fatalf("Filter() = %+v, want the original value %+v unchanged", got, u)
	}
}

func TestFilter_NilData(t *testing.T) {
	got, err := filter.Filter(nil, []string{"id"}, nil)
	if err != nil || got != nil {
		t.Fatalf("Filter(nil, ...) = %v, %v, want nil, nil", got, err)
	}
}

func TestFilter_NilPointer(t *testing.T) {
	var u *user
	got, err := filter.Filter(u, []string{"id"}, nil)
	if err != nil || got != nil {
		t.Fatalf("Filter(nil pointer, ...) = %v, %v, want nil, nil", got, err)
	}
}

func TestFilter_IncludeTopLevelFields(t *testing.T) {
	got, err := filter.Filter(sampleUser(), []string{"id", "name"}, nil)
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}

	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("Filter() returned %T, want map[string]any", got)
	}

	want := map[string]any{"id": 1, "name": "Jane"}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("Filter() = %+v, want %+v", m, want)
	}
}

func TestFilter_OmitTopLevelFields(t *testing.T) {
	got, err := filter.Filter(sampleUser(), nil, []string{"email", "tags", "address"})
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}

	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("Filter() returned %T, want map[string]any", got)
	}

	if _, present := m["email"]; present {
		t.Fatal("omitted field \"email\" is still present")
	}
	if _, present := m["id"]; !present {
		t.Fatal("non-omitted field \"id\" is missing")
	}
}

func TestFilter_OmitEmptyIsRespected(t *testing.T) {
	// Bio has `json:"bio,omitempty"` and is left at its zero value, so it
	// should never appear even without being explicitly omitted.
	got, err := filter.Filter(sampleUser(), nil, nil)
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}
	if got.(user).Bio != "" {
		t.Fatal("test setup invariant broken: Bio should be empty")
	}

	got, err = filter.Filter(sampleUser(), []string{"id", "bio"}, nil)
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}
	m := got.(map[string]any)
	if _, present := m["bio"]; present {
		t.Fatal("an empty omitempty field should be dropped even when explicitly included")
	}
}

func TestFilter_NestedDotPath(t *testing.T) {
	got, err := filter.Filter(sampleUser(), []string{"id", "address.city"}, nil)
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}

	m := got.(map[string]any)
	want := map[string]any{
		"id":      1,
		"address": map[string]any{"city": "Hanoi"},
	}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("Filter() = %+v, want %+v", m, want)
	}
}

func TestFilter_ProjectionSyntax(t *testing.T) {
	// "address{city}" is equivalent to "address.city" via the {..} form.
	got, err := filter.Filter(sampleUser(), []string{"id", "address{city}"}, nil)
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}

	m := got.(map[string]any)
	want := map[string]any{
		"id":      1,
		"address": map[string]any{"city": "Hanoi"},
	}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("Filter() = %+v, want %+v", m, want)
	}
}

func TestFilter_SliceIndex(t *testing.T) {
	// "tags[0]" selects that single element itself, not a 1-item slice.
	got, err := filter.Filter(sampleUser(), []string{"tags[0]"}, nil)
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}

	m := got.(map[string]any)
	want := map[string]any{"tags": "admin"}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("Filter() = %+v, want %+v", m, want)
	}
}

func TestFilter_SliceOutOfRangeIndex(t *testing.T) {
	got, err := filter.Filter(sampleUser(), []string{"tags[99]"}, nil)
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}

	m := got.(map[string]any)
	if m["tags"] != nil {
		t.Fatalf(`Filter()["tags"] = %v, want nil for an out-of-range index`, m["tags"])
	}
}

func TestFilter_OmitSliceIndex(t *testing.T) {
	// omit "tags[0]" drops that single slice element.
	got, err := filter.Filter(sampleUser(), nil, []string{"tags[0]"})
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}

	m := got.(map[string]any)
	want := []any{"beta"}
	if !reflect.DeepEqual(m["tags"], want) {
		t.Fatalf(`Filter()["tags"] = %+v, want %+v`, m["tags"], want)
	}
}

func TestFilter_MultiFieldProjection(t *testing.T) {
	// "address{city,street}" projects multiple fields at once.
	got, err := filter.Filter(sampleUser(), []string{"address{city,street}"}, nil)
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}

	m := got.(map[string]any)
	want := map[string]any{
		"address": map[string]any{"city": "Hanoi", "street": "Ba Trieu"},
	}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("Filter() = %+v, want %+v", m, want)
	}
}

func TestFilter_OmitEmptyNonStringKinds(t *testing.T) {
	// Ensure omitempty numeric/bool zero-values are omitted.
	got, err := filter.Filter(sampleUser(), []string{"id", "login_count", "verified"}, nil)
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}

	m := got.(map[string]any)
	if _, present := m["login_count"]; present {
		t.Fatal("a zero-value omitempty int field should be dropped")
	}
	if _, present := m["verified"]; present {
		t.Fatal("a zero-value omitempty bool field should be dropped")
	}
}

func TestFilter_MapValue(t *testing.T) {
	data := map[string]any{"a": 1, "b": 2, "c": 3}

	got, err := filter.Filter(data, []string{"a", "c"}, nil)
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}

	want := map[string]any{"a": 1, "c": 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Filter() = %+v, want %+v", got, want)
	}
}

func TestFilter_IncludeAndOmitCombined(t *testing.T) {
	// fields selects {id, address}, then omit removes address.street from
	// what fields already kept.
	got, err := filter.Filter(sampleUser(), []string{"id", "address"}, []string{"address.street"})
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}

	m := got.(map[string]any)
	want := map[string]any{
		"id":      1,
		"address": map[string]any{"city": "Hanoi"},
	}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("Filter() = %+v, want %+v", m, want)
	}
}

func TestFilter_UnexportedFieldsAreSkipped(t *testing.T) {
	// omit-only mode traverses unexported fields safely without panic.
	got, err := filter.Filter(sampleUser(), nil, []string{"email"})
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}

	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("Filter() returned %T, want map[string]any", got)
	}
	if _, present := m["private"]; present {
		t.Fatal("unexported field \"private\" should never appear in the result")
	}
	if _, present := m["id"]; !present {
		t.Fatal("exported field \"id\" should still be present")
	}
}
