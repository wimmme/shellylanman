package shellylanman

import (
	"os"
	"strings"
	"testing"
)

// The README's compose example is docker-compose.yml itself, so the options
// documented there cannot drift apart (DECISIONS P17-5).
func TestReadmeComposeExample(t *testing.T) {
	compose, err := os.ReadFile("docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	block := "```yaml\n" + strings.TrimRight(string(compose), "\n") + "\n```"
	if !strings.Contains(string(readme), block) {
		t.Fatal("README.md: the compose example differs from docker-compose.yml — copy the file into the Quick start")
	}
}
