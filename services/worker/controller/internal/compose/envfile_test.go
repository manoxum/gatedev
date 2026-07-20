package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnvLineUnquotesValues(t *testing.T) {
	key, value, ok := parseEnvLine(`DISCOVER_NODE_NAME="Daniel Costa"`)
	if !ok || key != "DISCOVER_NODE_NAME" || value != "Daniel Costa" {
		t.Fatalf("parseEnvLine returned (%q, %q, %v), want unquoted Daniel Costa", key, value, ok)
	}
}

func TestUpdateEnvKeysQuotesValuesWithSpaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.main")
	if err := os.WriteFile(path, []byte("DISCOVER_NODE_NAME=old\nDISCOVER_PORT=8531\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := updateEnvKeys(path, map[string]string{"DISCOVER_NODE_NAME": "Daniel Costa"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "DISCOVER_NODE_NAME=\"Daniel Costa\"\nDISCOVER_PORT=8531\n" {
		t.Fatalf("updated env = %q", got)
	}
}

func TestReadEnvValuesReturnsUnquotedValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.main")
	if err := os.WriteFile(path, []byte("DISCOVER_NODE_NAME=\"Daniel Costa\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	values, err := readEnvValues(path, []string{"DISCOVER_NODE_NAME"})
	if err != nil {
		t.Fatal(err)
	}
	if values["DISCOVER_NODE_NAME"] != "Daniel Costa" {
		t.Fatalf("DISCOVER_NODE_NAME = %q, want Daniel Costa", values["DISCOVER_NODE_NAME"])
	}
}
