package contracts

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/model"
)

type schema struct {
	Type       any               `json:"type"`
	Properties map[string]schema `json:"properties"`
	Required   []string          `json:"required"`
	Items      *schema           `json:"items"`
}

func TestGoWireModelMatchesRepositorySchema(t *testing.T) {
	b, err := os.ReadFile("../../shared/api/overview.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var s schema
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	checkType(t, "Overview", reflect.TypeOf(model.Overview{}), s)
}

func checkType(t *testing.T, path string, typ reflect.Type, s schema) {
	t.Helper()
	if typ == reflect.TypeOf(time.Time{}) {
		if s.Type != "string" {
			t.Errorf("%s timestamp must be string", path)
		}
		return
	}
	if typ.Kind() == reflect.Pointer {
		types, ok := s.Type.([]any)
		if !ok || len(types) != 2 || types[1] != "null" {
			t.Fatalf("%s pointer must explicitly allow null", path)
		}
		s.Type = types[0]
		checkType(t, path, typ.Elem(), s)
		return
	}
	switch typ.Kind() {
	case reflect.Struct:
		if s.Type != "object" {
			t.Fatalf("%s must be object", path)
		}
		if len(s.Properties) != typ.NumField() || len(s.Required) != typ.NumField() {
			t.Fatalf("%s fields differ between Go/schema", path)
		}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			name := f.Tag.Get("json")
			if name == "" || strings.Contains(name, ",") {
				t.Fatalf("%s.%s must have a required JSON field", path, f.Name)
			}
			sub, ok := s.Properties[name]
			if !ok {
				t.Fatalf("schema missing %s.%s", path, name)
			}
			required := false
			for _, key := range s.Required {
				if key == name {
					required = true
				}
			}
			if !required {
				t.Fatalf("%s.%s not required", path, name)
			}
			checkType(t, path+"."+name, f.Type, sub)
		}
	case reflect.Slice:
		if s.Type != "array" || s.Items == nil {
			t.Fatalf("%s must be array", path)
		}
		checkType(t, path+"[]", typ.Elem(), *s.Items)
	case reflect.String:
		if s.Type != "string" {
			t.Errorf("%s must be string", path)
		}
	case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64:
		if s.Type != "integer" && s.Type != "number" {
			t.Errorf("%s must be numeric", path)
		}
	case reflect.Float32, reflect.Float64:
		if s.Type != "number" {
			t.Errorf("%s must be number", path)
		}
	default:
		t.Fatalf("unhandled Go kind %s at %s", typ.Kind(), path)
	}
}
