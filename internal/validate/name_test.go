package validate

import "testing"

func TestName(t *testing.T) {
	valid := []string{
		"acme",
		"acme-corp",
		"host-001",
		"a",
		"123",
		"a-b-c",
	}
	for _, name := range valid {
		if err := Name(name); err != nil {
			t.Errorf("Name(%q) should be valid, got: %v", name, err)
		}
	}

	invalid := []string{
		"",
		"ACME",
		"Acme Corp",
		"acme corp",
		"-starts-with-dash",
		"has_underscore",
		"has.dot",
		"has/slash",
		"UPPER",
		string(make([]byte, 64)), // 64 chars (over limit)
	}
	for _, name := range invalid {
		if err := Name(name); err == nil {
			t.Errorf("Name(%q) should be invalid, got nil", name)
		}
	}
}
