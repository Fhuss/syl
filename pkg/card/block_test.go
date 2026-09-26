package card

import (
	"strings"
	"testing"
)

func TestExtractActionBlocks(t *testing.T) {
	markdown := `
Here is how to deploy your service:

1. Build the docker container:
` + "```bash\ndocker build -t myapp:latest .\n```" + `

2. Run database migration with ` + "`pnpm prisma migrate deploy`" + ` before starting.

3. Verify the deployment at https://dashboard.example.com/status.

` + "```sql\nSELECT * FROM users WHERE active = true;\n```" + `
`

	blocks := ExtractActionBlocks(markdown)
	if len(blocks) != 4 {
		t.Fatalf("expected 4 blocks, got %d", len(blocks))
	}

	// 1. First block: Bash code
	if blocks[0].Type != BlockCode || blocks[0].Badge != "BASH" || blocks[0].Key != "1" {
		t.Errorf("block 0: expected BASH code with key 1, got %+v", blocks[0])
	}
	if blocks[0].Content != "docker build -t myapp:latest ." {
		t.Errorf("block 0 content mismatch: %q", blocks[0].Content)
	}

	// 2. Second block: SQL code (code blocks come first)
	if blocks[1].Type != BlockCode || blocks[1].Badge != "SQL" || blocks[1].Key != "2" {
		t.Errorf("block 1: expected SQL code with key 2, got %+v", blocks[1])
	}

	// 3. Third block: Step 2 with inline command
	if blocks[2].Type != BlockStep || blocks[2].Key != "3" {
		t.Errorf("block 2: expected Step with key 3, got %+v", blocks[2])
	}
	if blocks[2].Content != "pnpm prisma migrate deploy" {
		t.Errorf("block 2 content mismatch: %q", blocks[2].Content)
	}

	// 4. Fourth block: URL
	if blocks[3].Type != BlockURL || blocks[3].Key != "4" {
		t.Errorf("block 3: expected URL with key 4, got %+v", blocks[3])
	}
	if blocks[3].Content != "https://dashboard.example.com/status" {
		t.Errorf("block 3 content mismatch: %q", blocks[3].Content)
	}
}

func TestExtractActionBlocksKeyLimits(t *testing.T) {
	content := ""
	for i := 1; i <= 12; i++ {
		content += "```\necho " + string(rune('0'+i)) + "\n```\n"
	}

	blocks := ExtractActionBlocks(content)
	if len(blocks) != 12 {
		t.Fatalf("expected 12 blocks, got %d", len(blocks))
	}
	if blocks[0].Key != "1" {
		t.Errorf("expected key 1, got %q", blocks[0].Key)
	}
	if blocks[8].Key != "9" {
		t.Errorf("expected key 9, got %q", blocks[8].Key)
	}
	if blocks[9].Key != "0" {
		t.Errorf("expected key 0, got %q", blocks[9].Key)
	}
	if blocks[10].Key != "" {
		t.Errorf("expected empty key for block 11, got %q", blocks[10].Key)
	}
}

func TestExtractActionBlocksWithFiles(t *testing.T) {
	markdown := `
Review the configuration files:
- Inspect the server entry: [server.go](file:///Users/fabian/Dev/syl/cmd/syl/main.go#L1-L20)
- Check ~/.config/syl/config.yaml for provider settings.
- The build script is located at /etc/nginx/nginx.conf.
`

	blocks := ExtractActionBlocks(markdown)
	if len(blocks) < 3 {
		t.Fatalf("expected at least 3 file blocks, got %d", len(blocks))
	}

	foundGo := false
	foundYaml := false
	foundConf := false

	for _, b := range blocks {
		if b.Type == BlockFile {
			switch b.Badge {
			case "GO":
				foundGo = true
				if b.Content != "/Users/fabian/Dev/syl/cmd/syl/main.go" {
					t.Errorf("expected stripped line fragment, got %q", b.Content)
				}
			case "YAML":
				foundYaml = true
			case "CONF":
				foundConf = true
				if b.Content != "/etc/nginx/nginx.conf" {
					t.Errorf("expected /etc/nginx/nginx.conf, got %q", b.Content)
				}
			}
		}
	}

	if !foundGo {
		t.Errorf("expected to extract GO file block")
	}
	if !foundYaml {
		t.Errorf("expected to extract YAML file block")
	}
	if !foundConf {
		t.Errorf("expected to extract CONF file block")
	}
}

func TestFileBadgeResolution(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/app/main.go", "GO"},
		{"/app/script.py", "PY"},
		{"/app/index.ts", "TS"},
		{"/app/Dockerfile", "DOCKER"},
		{"/app/Makefile", "MAKE"},
		{"/app/unknown.xyz", "FILE"},
	}

	for _, tt := range tests {
		got := fileBadge(tt.path)
		if got != tt.expected {
			t.Errorf("fileBadge(%q) = %q, expected %q", tt.path, got, tt.expected)
		}
	}
}

func TestToggleTaskCheckbox(t *testing.T) {
	content := `- [ ] Step 1: Install deps
- [ ] Step 2: Migrate DB`

	// 1. Toggle first task
	c1, ok, toast1 := ToggleTaskCheckbox(content)
	if !ok || !strings.Contains(c1, "- [x] Step 1: Install deps") {
		t.Fatalf("expected step 1 checked, got: %s", c1)
	}
	if !strings.Contains(toast1, "Completed: Step 1") {
		t.Errorf("unexpected toast: %s", toast1)
	}

	// 2. Toggle second task
	c2, ok, toast2 := ToggleTaskCheckbox(c1)
	if !ok || !strings.Contains(c2, "- [x] Step 2: Migrate DB") {
		t.Fatalf("expected step 2 checked, got: %s", c2)
	}
	if !strings.Contains(toast2, "Completed: Step 2") {
		t.Errorf("unexpected toast: %s", toast2)
	}

	// 3. When all checked, reset to unchecked
	c3, ok, toast3 := ToggleTaskCheckbox(c2)
	if !ok || strings.Contains(c3, "- [x]") {
		t.Fatalf("expected all reset to unchecked, got: %s", c3)
	}
	if !strings.Contains(toast3, "Reset") {
		t.Errorf("unexpected reset toast: %s", toast3)
	}
}

func TestFoldCodeBlocks(t *testing.T) {
	content := `Here is the setup:
` + "```bash\nline 1\nline 2\nline 3\n```" + `
Done!`

	folded := FoldCodeBlocks(content)
	if !strings.Contains(folded, "folded, 'za' to expand") {
		t.Fatalf("expected folded indicator, got: %s", folded)
	}
	if strings.Contains(folded, "line 2") {
		t.Fatalf("expected code body to be folded away, got: %s", folded)
	}
}

