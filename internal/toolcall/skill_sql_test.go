package toolcall

import (
	"os"
	"testing"

	"gitcode.com/dscli/dscli/internal/sqlite"
)

func TestGetProjectSkills(t *testing.T) {
	dbPath := sqlite.GetDBPath()
	testPath := dbPath + ".test.db"
	sqlite.SetDBPath(testPath)
	t.Cleanup(func() {
		sqlite.SetDBPath(dbPath)
		os.RemoveAll(testPath)
	})
	goModernizeSkill := &Skill{
		Name:        "GoModernize",
		Description: "suggest simplifications to Go code, using modern language and library features",
		Content: `Each diagnostic provides a fix. Our intent is that these fixes may be safely applied 
en masse without changing the behavior of your program. In some cases the suggested fixes are imperfect
 and may lead to (for example) unused imports or unused local variables, causing build breakage. However, 
these problems are generally trivial to fix. 
We regard any modernizer whose fix changes program behavior to have a serious bug and will 
endeavor to fix it.

To apply all modernization fixes en masse, you can use the following command:
modernize -fix ./...
`,
		Category: "Programming",
	}
	err := CreateSkill(t.Context(), goModernizeSkill)
	if err != nil {
		t.Fatal(err)
	}
	if goModernizeSkill.ID == 0 {
		t.Fatal(goModernizeSkill)
	}

	err = CreateProjectSkill(t.Context(), goModernizeSkill.ID)
	if err != nil {
		t.Fatal(err)
	}
	skills, err := GetProjectSkills(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if l := len(skills); l != 1 {
		t.Fatal(l)
	}
}
