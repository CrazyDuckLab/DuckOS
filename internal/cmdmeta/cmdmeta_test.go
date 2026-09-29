// cmdmeta/cmdmeta_test.go pins the V1
// "Show CLI / Show API" metadata
// registry.
package cmdmeta

import "testing"

func TestEntryValid(t *testing.T) {
	good := Entry{Operation: "show.interface", Object: "interface", Kind: OpShow, Summary: "show interface", CLITemplate: "duck show interface {{name}}", APIMethod: "GET", APIPath: "/v1/interfaces/{{name}}"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noOp := Entry{Operation: "BAD", Object: "interface", Kind: OpShow, Summary: "x", CLITemplate: "x"}
	if err := noOp.Valid(); err == nil {
		t.Fatal("non-canonical op must fail")
	}
	noObj := Entry{Operation: "show.interface", Object: "1bad", Kind: OpShow, Summary: "x", CLITemplate: "x"}
	if err := noObj.Valid(); err == nil {
		t.Fatal("non-canonical object must fail")
	}
	noSummary := Entry{Operation: "show.interface", Object: "interface", Kind: OpShow, CLITemplate: "x"}
	if err := noSummary.Valid(); err == nil {
		t.Fatal("missing summary must fail")
	}
	noKind := Entry{Operation: "show.interface", Object: "interface", Summary: "x", CLITemplate: "x"}
	if err := noKind.Valid(); err == nil {
		t.Fatal("unknown kind must fail")
	}
	apiPathNoMethod := Entry{Operation: "show.interface", Object: "interface", Kind: OpShow, Summary: "x", APIPath: "/x"}
	if err := apiPathNoMethod.Valid(); err == nil {
		t.Fatal("apiPath without apiMethod must fail")
	}
}

func TestRegistryCRUD(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(Entry{Operation: "show.interface", Object: "interface", Kind: OpShow, Summary: "x", CLITemplate: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(Entry{Operation: "set.interface", Object: "interface", Kind: OpSet, Summary: "x", CLITemplate: "x"}); err != nil {
		t.Fatal(err)
	}
	if len(r.List()) != 2 {
		t.Fatalf("list: %d", len(r.List()))
	}
	if got, ok := r.Get("show.interface"); !ok || got.Operation != "show.interface" {
		t.Fatal("get failed")
	}
	if got := r.ByObject("interface"); len(got) != 2 {
		t.Fatalf("by object: %d", len(got))
	}
}

func TestRenderCLI(t *testing.T) {
	e := Entry{Operation: "show.interface", Object: "interface", Kind: OpShow, Summary: "x", CLITemplate: "duck show interface {{name}}"}
	out, err := RenderCLI(e, map[string]string{"name": "eth0"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "duck show interface eth0" {
		t.Fatalf("rendered: %q", out)
	}
}

func TestRenderCLIMultiple(t *testing.T) {
	e := Entry{Operation: "x", Object: "y", Kind: OpShow, Summary: "x", CLITemplate: "duck {{a}} {{b}} {{a}}"}
	out, _ := RenderCLI(e, map[string]string{"a": "1", "b": "2"})
	if out != "duck 1 2 1" {
		t.Fatalf("rendered: %q", out)
	}
}
