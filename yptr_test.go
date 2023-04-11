package yptr_test

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"

	yptr "github.com/zachelrath/yaml-jsonpointer"
	yaml "gopkg.in/yaml.v3"
)

func ExampleFind() {
	src := `
a:
  b:
    c: 42
d:
- e
- f
`
	var n yaml.Node
	yaml.Unmarshal([]byte(src), &n)

	r, _ := yptr.Find(&n, `/a/b/c`)
	fmt.Printf("Scalar %q at %d:%d\n", r.Value, r.Line, r.Column)

	r, _ = yptr.Find(&n, `/d/0`)
	fmt.Printf("Scalar %q at %d:%d\n", r.Value, r.Line, r.Column)
	// Output: Scalar "42" at 4:8
	// Scalar "e" at 6:3
}

func ExampleFind_extension() {
	src := `kind: Deployment
apiVersion: apps/v1
metadata:
  name: foo
spec:
  template:
    spec:
      replicas: 1
      containers:
      - name: app
        image: nginx
      - name: sidecar
        image: mysidecar
`
	var n yaml.Node
	yaml.Unmarshal([]byte(src), &n)

	r, _ := yptr.Find(&n, `/spec/template/spec/containers/1/image`)
	fmt.Printf("Scalar %q at %d:%d\n", r.Value, r.Line, r.Column)

	r, _ = yptr.Find(&n, `/spec/template/spec/containers/~{"name":"app"}/image`)
	fmt.Printf("Scalar %q at %d:%d\n", r.Value, r.Line, r.Column)

	// Output: Scalar "mysidecar" at 13:16
	// Scalar "nginx" at 11:16
}

func ExampleFind_jsonPointerCompat() {
	// the array item match syntax doesn't accidentally match a field that just happens
	// to contain the same characters.
	src := `a:
  "{\"b\":\"c\"}": d
`
	var n yaml.Node
	yaml.Unmarshal([]byte(src), &n)

	r, _ := yptr.Find(&n, `/a/{"b":"c"}`)

	fmt.Printf("Scalar %q at %d:%d\n", r.Value, r.Line, r.Column)

	// Output: Scalar "d" at 2:20
}

func TestParse(t *testing.T) {
	src := `
spec:
  template:
    spec:
      replicas: 1
      containers:
      - name: app
        image: nginx
      - name: cache
        image: redis
        tag: 6.3.2
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(src), &root); err != nil {
		t.Fatal(err)
	}
	if _, err := yptr.Find(&root, "/bad/path"); !errors.Is(err, yptr.ErrNotFound) {
		t.Fatalf("expecting not found error, got: %v", err)
	}

	testCases := []struct {
		ptr    string
		value  string
		line   int
		column int
	}{
		{`/spec/template/spec/replicas`, "1", 5, 17},
		{`/spec/template/spec/containers/0/image`, "nginx", 8, 16},
		{`/spec/template/spec/containers/~{"name":"app"}/image`, "nginx", 8, 16},
		{`/spec/template/spec/containers/~{"name":"cache"}/tag`, "6.3.2", 11, 14},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			r, err := yptr.Find(&root, tc.ptr)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := r.Value, tc.value; got != want {
				t.Fatalf("got: %v, want: %v", got, want)
			}
			if got, want := r.Line, tc.line; got != want {
				t.Errorf("got: %v, want: %v", got, want)
			}
			if got, want := r.Column, tc.column; got != want {
				t.Errorf("got: %v, want: %v", got, want)
			}
		})
	}

	errorCases := []struct {
		ptr string
		err error
	}{
		{"a", fmt.Errorf(`JSON pointer must be empty or start with a "/`)},
		{"/a", yptr.ErrNotFound},
	}
	for i, tc := range errorCases {
		t.Run(fmt.Sprint("error", i), func(t *testing.T) {
			_, err := yptr.Find(&root, tc.ptr)
			if err == nil {
				t.Fatal("error expected")
			}
			if got, want := err, tc.err; got.Error() != want.Error() && !errors.Is(got, want) {
				t.Errorf("got: %v, want: %v", got, want)
			}
		})
	}
}

func TestFindAll(t *testing.T) {
	src := `
spec:
  templates:
  - replicas: 1
    containers:
    - name: app
      image: nginx
    - name: cache
      image: redis
      tag: 6.3.2
  - replicas: 2
    containers:
    - name: db
      image: postgres
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(src), &root); err != nil {
		t.Fatal(err)
	}
	// If a path is not found, no error should be returned, just an empty array
	results, err := yptr.FindAll(&root, "/bad/path")
	if err != nil {
		t.Fatalf("expecting no error, got: %v", err)
	}
	if len(results) > 0 {
		t.Fatalf("expecting empty results array, got: %v", results)
	}

	testCases := []struct {
		ptr  string
		want []string
	}{
		{`/spec/templates/~{}/containers/0/image`, []string{"nginx", "postgres"}},
		{`/spec/templates/~{}/replicas`, []string{"1", "2"}},
		{`/spec/templates/~{}/containers/~{}/tag`, []string{"6.3.2"}},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			results, err := yptr.FindAll(&root, tc.ptr)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, len(results))
			for i, r := range results {
				got[i] = r.Value
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got: %v, want: %v", got, tc.want)
			}
		})
	}
}

func TestFindAllStrict(t *testing.T) {
	src := `
spec:
  templates:
  - replicas: 1
    apiVersion: v1beta
    containers:
    - name: app
      image: nginx
    - name: cache
      image: redis
      tag: 6.3.2
  - replicas: 2
    apiVersion: v1
    containers:
    - name: db
      image: postgres
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(src), &root); err != nil {
		t.Fatal(err)
	}
	// If a path is not found, ErrNotFound should be returned
	if _, err := yptr.FindAllStrict(&root, "/bad/path"); !errors.Is(err, yptr.ErrNotFound) {
		t.Fatalf("expecting ErrNotFound, got: %v", err)
	}

	testCases := []struct {
		ptr  string
		want []string
	}{
		{`/spec/templates/0/containers/1/image`, []string{"redis"}},
		{`/spec/templates/~{}/replicas`, []string{"1", "2"}},
		{`/spec/templates/~{"apiVersion":"v1beta"}/containers/~{}/image`, []string{"nginx", "redis"}},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			results, err := yptr.FindAllStrict(&root, tc.ptr)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, len(results))
			for i, r := range results {
				got[i] = r.Value
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got: %v, want: %v", got, tc.want)
			}
		})
	}
}
