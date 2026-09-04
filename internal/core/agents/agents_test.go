package agents

import (
	"os"
	"path/filepath"
	"testing"
)

func isolate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("CHARMULET_CONFIG_DIR", root)
	return root
}

func TestAgentRoundtrip(t *testing.T) {
	isolate(t)

	a := Agent{Name: "nightowl", Model: "qwen3.5:9b", SystemPrompt: "You code at night.", Wrapper: ""}
	if err := Save(a); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := Save(Agent{Name: "beta", Model: "gemma4:e4b", SystemPrompt: "Second agent."}); err != nil {
		t.Fatalf("save beta: %v", err)
	}

	list, err := List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].Name != "beta" || list[1].Name != "nightowl" {
		t.Fatalf("list wrong: %+v", list)
	}

	// Corrupt file is skipped, not fatal.
	d, _ := Dir()
	if err := os.WriteFile(filepath.Join(d, "broken.toml"), []byte("not [ toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err = List()
	if err != nil || len(list) != 2 {
		t.Fatalf("corrupt file should be skipped: %d %v", len(list), err)
	}

	// Rename-on-save: save under new name then Remove old.
	if err := Save(Agent{Name: "nightowl2", Model: "x", SystemPrompt: "You code at night."}); err != nil {
		t.Fatal(err)
	}
	if err := Remove("nightowl"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := Remove("nightowl"); err != nil { // idempotent
		t.Fatalf("remove twice: %v", err)
	}
	list, _ = List()
	if len(list) != 2 {
		t.Fatalf("post-remove list: %+v", list)
	}

	// Path traversal rejected.
	if err := Save(Agent{Name: "../escape", Model: "x"}); err == nil {
		t.Fatal("traversal save should fail")
	}
}

func TestSkillsLifecycle(t *testing.T) {
	isolate(t)

	body := "---\nname = \"wat2do\"\ndescription = \"daily missions helper\"\n---\n\nDo the thing."
	if err := WriteSkill("wat2do", body); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := WriteSkill("plain", "# Plain\n\nFirst body line is the desc."); err != nil {
		t.Fatalf("write 2: %v", err)
	}

	skills, err := ListSkills()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("want 2 skills, got %d", len(skills))
	}
	if skills[0].Name != "plain" || skills[0].Desc != "First body line is the desc." || !skills[0].Enabled {
		t.Fatalf("plain skill parsed wrong: %+v", skills[0])
	}
	if skills[1].Name != "wat2do" || skills[1].Desc != "daily missions helper" || !skills[1].Enabled {
		t.Fatalf("frontmatter skill parsed wrong: %+v", skills[1])
	}

	// Toggle off and back on via rename.
	off, err := ToggleSkill(skills[1])
	if err != nil || off.Enabled {
		t.Fatalf("disable: %+v %v", off, err)
	}
	again, _ := ListSkills()
	if len(again) != 2 || again[1].Enabled {
		t.Fatalf("disabled state not listed: %+v", again)
	}
	on, err := ToggleSkill(off)
	if err != nil || !on.Enabled {
		t.Fatalf("re-enable: %+v %v", on, err)
	}

	// Invalid name rejected.
	if err := WriteSkill("../evil", "x"); err == nil {
		t.Fatal("traversal write should fail")
	}
}
