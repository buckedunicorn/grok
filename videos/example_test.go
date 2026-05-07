package videos_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/videos"
)

func ExampleClient_Generate() {
	client := grok.New()

	// Generate returns a request_id immediately; the video renders asynchronously.
	requestID, err := client.Videos.Generate(context.Background(), &videos.GenerateRequest{
		Prompt: "A time-lapse of a city waking up at dawn",
	})
	if err != nil {
		log.Fatal(err)
	}

	// Wait polls until the video is ready or an error occurs.
	result, err := client.Videos.Wait(context.Background(), requestID, 5*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Video.URL)
}
