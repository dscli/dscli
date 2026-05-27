package parse

import (
	"encoding/json"
	"os"
	"testing"
)

func TestCStructure(t *testing.T) {
	content := `#include <stdio.h>
#include <stdlib.h>

#define MAX_SIZE 1024

struct point {
    int x;
    int y;
};

typedef struct point Point;

enum color { RED, GREEN, BLUE };

int add(int a, int b) {
    return a + b;
}

void print_point(Point *p) {
    printf("(%d, %d)\n", p->x, p->y);
}

int main(void) {
    int result = add(3, 4);
    printf("result = %d\n", result);
    return 0;
}
`
	err := os.WriteFile("test_c.c", []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove("test_c.c")

	fs, err := ParseFileStructure(t.Context(), "test_c.c")
	if err != nil {
		t.Fatalf("ParseFileStructure failed: %v", err)
	}

	b, _ := json.MarshalIndent(fs, "", "  ")
	t.Logf("Result:\n%s", string(b))

	// Verify language
	if fs.Language != "c" {
		t.Errorf("expected language 'c', got %q", fs.Language)
	}

	// Verify function names
	wantFuncs := map[string]bool{"add": true, "print_point": true, "main": true}
	for _, f := range fs.Functions {
		if !wantFuncs[f.Name] {
			t.Errorf("unexpected function: name=%q", f.Name)
		}
		delete(wantFuncs, f.Name)
	}
	for name := range wantFuncs {
		t.Errorf("missing function: %q", name)
	}

	// Verify class names and types
	wantClasses := map[string]string{
		"point":    "struct",
		"Point":    "typedef",
		"color":    "enum",
		"MAX_SIZE": "macro",
	}
	for _, c := range fs.Classes {
		wantType, ok := wantClasses[c.Name]
		if !ok {
			t.Errorf("unexpected class: name=%q type=%q", c.Name, c.Type)
			continue
		}
		if c.Type != wantType {
			t.Errorf("class %q: expected type %q, got %q", c.Name, wantType, c.Type)
		}
		delete(wantClasses, c.Name)
	}
	for name, typ := range wantClasses {
		t.Errorf("missing class: %q (type=%q)", name, typ)
	}

	// Verify includes
	wantIncludes := map[string]bool{"<stdio.h>": true, "<stdlib.h>": true}
	for _, imp := range fs.Imports {
		if !wantIncludes[imp] {
			t.Errorf("unexpected import: %q", imp)
		}
		delete(wantIncludes, imp)
	}
	for imp := range wantIncludes {
		t.Errorf("missing import: %q", imp)
	}
}
