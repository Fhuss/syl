package card

import (
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
