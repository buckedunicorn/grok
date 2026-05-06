// files demonstrates uploading, listing (with the All iterator), and cleaning up files.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/buckedunicorn/grok"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))
	ctx := context.Background()

	// Write a temp JSONL file so we can demonstrate UploadPath.
	tmp, err := os.CreateTemp("", "grok-example-*.jsonl")
	if err != nil {
		fmt.Fprintln(os.Stderr, "tempfile:", err)
		os.Exit(1)
	}
	_, _ = fmt.Fprintln(tmp, `{"messages":[{"role":"user","content":"Hello"},{"role":"assistant","content":"Hi!"}]}`)
	tmp.Close()
	defer os.Remove(tmp.Name())

	// UploadPath opens the file by path, no manual os.Open required.
	f, err := client.Files.UploadPath(ctx, tmp.Name())
	if err != nil {
		fmt.Fprintln(os.Stderr, "upload:", err)
		os.Exit(1)
	}
	fmt.Printf("uploaded: id=%s filename=%s bytes=%d\n", f.ID, f.Filename, f.Bytes)

	// All iterates through every page automatically, break early to stop.
	fmt.Println("\nall files:")
	count := 0
	for file, err := range client.Files.All(ctx, nil) {
		if err != nil {
			fmt.Fprintln(os.Stderr, "list:", err)
			os.Exit(1)
		}
		fmt.Printf("  %s  %s  %d bytes\n", file.ID, file.Filename, file.Bytes)
		count++
	}
	fmt.Printf("total: %d\n", count)

	// Clean up the file we uploaded.
	if err := client.Files.Delete(ctx, f.ID); err != nil {
		fmt.Fprintln(os.Stderr, "delete:", err)
		os.Exit(1)
	}
	fmt.Println("\ndeleted:", f.ID)
}
