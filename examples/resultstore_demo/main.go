package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/richinex/davingo/storage"
)

func main() {
	// Clean up from previous runs
	os.RemoveAll(".poetry_store")

	// Open SQLite for ContentStorage
	db, err := storage.OpenSqlite(".poetry_store/metadata.db")
	if err != nil {
		fmt.Printf("Failed to open database: %v\n", err)
		return
	}
	defer db.Close()

	// Create ResultStore with SQLite backend
	store, err := storage.NewResultStore(db)
	if err != nil {
		fmt.Printf("Failed to create store: %v\n", err)
		return
	}
	defer store.Close()

	ctx := context.Background()
	sessionID := "poetry-session"

	fmt.Println("=== Poetry ResultStore Demo ===")
	fmt.Println()

	// Store poem 1: Short verse (stays in memory)
	poem1 := `The morning sun breaks through the haze,
A golden light on dusty days.
The city wakes with sounds of life,
Through joy and sorrow, peace and strife.
`
	key1 := storage.ResultKey{SessionID: sessionID, Key: "poems/morning_sun.txt"}
	meta1, _ := store.Store(ctx, key1, poem1, storage.DefaultStoreOptions())
	fmt.Printf("Stored: %s\n", key1.Key)
	fmt.Printf("  Lines: %d, Bytes: %d, Hash: %s\n\n", meta1.LineCount, meta1.ByteSize, meta1.ContentHash)

	// Store poem 2: Another short verse
	poem2 := `Stars above the township glow,
Silent watchers down below.
Dreams take flight on midnight air,
Hope lives on through every care.
`
	key2 := storage.ResultKey{SessionID: sessionID, Key: "poems/township_stars.txt"}
	meta2, _ := store.Store(ctx, key2, poem2, storage.DefaultStoreOptions())
	fmt.Printf("Stored: %s\n", key2.Key)
	fmt.Printf("  Lines: %d, Bytes: %d, Hash: %s\n\n", meta2.LineCount, meta2.ByteSize, meta2.ContentHash)

	// Store poem 3: Generate a large poem (will go to disk)
	var largePoem strings.Builder
	largePoem.WriteString("# The Eternal Cycle\n\n")
	themes := []string{"dawn", "morning", "noon", "dusk", "night", "dreams"}
	for i := 0; i < 200; i++ {
		theme := themes[i%len(themes)]
		largePoem.WriteString(fmt.Sprintf("Verse %d: The %s awakens still,\n", i+1, theme))
		largePoem.WriteString(fmt.Sprintf("Through valleys deep and over hill,\n"))
		largePoem.WriteString(fmt.Sprintf("The ancient song that time has sung,\n"))
		largePoem.WriteString(fmt.Sprintf("Since first the world was fresh and young.\n\n"))
	}

	key3 := storage.ResultKey{SessionID: sessionID, Key: "poems/eternal_cycle.txt"}
	meta3, _ := store.Store(ctx, key3, largePoem.String(), storage.DefaultStoreOptions())
	fmt.Printf("Stored: %s\n", key3.Key)
	fmt.Printf("  Lines: %d, Bytes: %d, Hash: %s\n", meta3.LineCount, meta3.ByteSize, meta3.ContentHash)
	fmt.Printf("  (Large file - stored on disk)\n\n")

	// Store a duplicate (same content as poem1)
	key4 := storage.ResultKey{SessionID: sessionID, Key: "poems/morning_copy.txt"}
	meta4, _ := store.Store(ctx, key4, poem1, storage.DefaultStoreOptions())
	fmt.Printf("Stored duplicate: %s\n", key4.Key)
	fmt.Printf("  Hash: %s (same as morning_sun.txt = deduplicated!)\n\n", meta4.ContentHash)

	// Search demo
	fmt.Println("--- Search for 'dreams' ---")
	matches, _ := store.Search(ctx, sessionID, "dreams", 10)
	for _, m := range matches {
		fmt.Printf("  %s:%d - %s\n", m.Key.Key, m.Line, strings.TrimSpace(m.Context))
	}
	fmt.Println()

	// GetLines demo
	fmt.Println("--- Get lines 1-5 of eternal_cycle.txt ---")
	lines, _ := store.GetLines(ctx, key3, storage.LineRange{Start: 1, End: 5})
	fmt.Println(lines)

	fmt.Println("=== Demo Complete ===")
	fmt.Println()
	fmt.Println("Test commands for another terminal:")
	fmt.Println()
	fmt.Println("  # View SQLite metadata")
	fmt.Println("  sqlite3 .poetry_store/metadata.db 'SELECT key, line_count, byte_size, content_hash FROM results;'")
	fmt.Println()
	fmt.Println("  # Check file storage (large files)")
	fmt.Println("  ls -la .poetry_store/content/")
	fmt.Println()
	fmt.Println("  # View large file content")
	fmt.Println("  head -20 .poetry_store/content/*")
}
