package config

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"testing"
)

// exampleKey matches a variable line in .env.example, commented out or not.
var exampleKey = regexp.MustCompile(`^#?\s*([A-Z][A-Z0-9_]*)=`)

// TestMain unsets every variable server/.env.example documents, so a developer's
// own .env (`npx dotenv run -- go test ./...`) can't leak into the defaults
// under test. Each test then sets what it needs with t.Setenv.
func TestMain(m *testing.M) {
	f, err := os.Open("../../../.env.example")
	if err != nil {
		fmt.Fprintln(os.Stderr, "config tests: read .env.example:", err)
		os.Exit(1)
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if match := exampleKey.FindStringSubmatch(sc.Text()); match != nil {
			_ = os.Unsetenv(match[1])
		}
	}
	_ = f.Close()
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "config tests: read .env.example:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
