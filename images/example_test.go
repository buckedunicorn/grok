package images_test

import (
	"context"
	"fmt"
	"log"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/images"
)

func ExampleClient_Generate() {
	client := grok.New()
	n := 1
	resp, err := client.Images.Generate(context.Background(), &images.GenerateRequest{
		Prompt: "A gopher surfing a wave of Go code, digital art",
		N:      &n,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.Data[0].URL)
}
