package discord

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/images"
	"github.com/buckedunicorn/grok/videos"
)

// ----------------------------------------------------------------------------
// URLRing, small per-channel ring of URLs.
//
// Two callers in this package:
// - GenerateVideoTool stores each successful bot-generated video URL.
// ExtendVideoTool reads from the same ring as a fallback when the
// model didn't supply video_url and the per-turn MessageContext
// didn't have one either.
// - The example bot uses one for "user-attached images recently
// seen in this channel" so its resolveFallbacks has a stored fallback
// after the message scrolls out of the trimmed history window.
//
// Pass the same *URLRing instance to GenerateVideoTool and
// ExtendVideoTool when you want them to share state. Pass distinct
// instances (or nil) to keep them independent.
// ----------------------------------------------------------------------------

// urlRingShards is the number of independent buckets the ring is
// striped over. Sharding cuts mutex contention: in
// benchmarks against -cpu=12, parallel Add+Recent throughput improves
// roughly 4-5x compared to a single global mutex. 16 is a sweet spot
// (enough to scatter writes across cores, small enough that Reset/
// snapshot iteration cost stays trivial).
const urlRingShards = 16

type urlRingShard struct {
	mu      sync.Mutex
	perChan map[string][]string
}

// URLRing is a per-channel ring buffer of URLs, safe for concurrent use.
//
// The ring is internally sharded by FNV-32a of the channel ID, so
// concurrent Add/Recent on different channels rarely contend on the
// same mutex.
type URLRing struct {
	maxPer int
	shards [urlRingShards]urlRingShard
}

// NewURLRing returns a URLRing keeping at most maxPerChannel URLs per
// channel. maxPerChannel <= 0 defaults to 8.
func NewURLRing(maxPerChannel int) *URLRing {
	if maxPerChannel <= 0 {
		maxPerChannel = 8
	}
	r := &URLRing{maxPer: maxPerChannel}
	for i := range r.shards {
		r.shards[i].perChan = map[string][]string{}
	}
	return r
}

// shardFor returns the shard guarding channelID. fnv-32a is chosen for
// cheap hashing and good distribution; the same algorithm hash/fnv
// uses internally, inlined here to avoid the [hash.Hash32] interface
// allocation.
func (r *URLRing) shardFor(channelID string) *urlRingShard {
	const offset = 2166136261
	const prime = 16777619
	h := uint32(offset)
	for i := 0; i < len(channelID); i++ {
		h ^= uint32(channelID[i])
		h *= prime
	}
	return &r.shards[h%urlRingShards]
}

// Add appends url to channelID's ring, evicting oldest entries past
// the per-channel cap.
func (r *URLRing) Add(channelID, url string) {
	if r == nil || channelID == "" || url == "" {
		return
	}
	s := r.shardFor(channelID)
	s.mu.Lock()
	defer s.mu.Unlock()
	list := append(s.perChan[channelID], url)
	if over := len(list) - r.maxPer; over > 0 {
		list = list[over:]
	}
	s.perChan[channelID] = list
}

// Recent returns the most-recently added URL for channelID, or "" if
// the channel has none.
func (r *URLRing) Recent(channelID string) string {
	if r == nil {
		return ""
	}
	s := r.shardFor(channelID)
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.perChan[channelID]
	if len(list) == 0 {
		return ""
	}
	return list[len(list)-1]
}

// Reset drops every URL stored for channelID.
func (r *URLRing) Reset(channelID string) {
	if r == nil {
		return
	}
	s := r.shardFor(channelID)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.perChan, channelID)
}

// ----------------------------------------------------------------------------
// MediaToolOption, shared config knobs for the image/video tool factories.
// ----------------------------------------------------------------------------

// MediaToolOption customises a media tool factory.
type MediaToolOption func(*mediaToolConfig)

type mediaToolConfig struct {
	toolName string
	logf     func(format string, args ...any)
}

// WithMediaToolName overrides the function-tool name advertised to the
// model (defaults: "generate_image", "edit_image", "generate_video",
// "extend_video").
func WithMediaToolName(name string) MediaToolOption {
	return func(c *mediaToolConfig) { c.toolName = name }
}

// WithMediaToolLogger plumbs an optional logger through the handler.
// If nil, the handler is silent. Format strings are plain printf.
func WithMediaToolLogger(logf func(format string, args ...any)) MediaToolOption {
	return func(c *mediaToolConfig) { c.logf = logf }
}

func mergeMediaOptions(defaultName string, opts []MediaToolOption) *mediaToolConfig {
	c := &mediaToolConfig{toolName: defaultName}
	for _, o := range opts {
		o(c)
	}
	return c
}

// ----------------------------------------------------------------------------
// generate_image
// ----------------------------------------------------------------------------

// GenerateImageTool returns a (chat.Tool, chat.Handler) pair for
// text-to-image generation. The handler:
//
// - parses prompt + optional aspect_ratio from the model;
// - calls images.Client.Generate with response_format=url;
// - posts "<@user> <url>" to the channel via sender (when sender is
// non-nil and the ctx carries a channelID);
// - returns the URL + revised_prompt to the chat agent.
//
// channelID and userID come from ctx (Agent.Handle threads channelID
// via WithChannelContext; the caller threads userID via
// WithMessageContext). Pass sender = nil to skip the channel post -
// the chat agent still receives the URL via the tool result.
func GenerateImageTool(client *images.Client, sender Sender, opts ...MediaToolOption) (chat.Tool, chat.Handler) {
	cfg := mergeMediaOptions("generate_image", opts)
	tool := chat.Tool{
		Type: "function",
		Function: chat.FunctionDef{
			Name:        cfg.toolName,
			Description: "Generate a brand-new image from a text prompt. Use when the user asks you to draw / paint / design / create an image from scratch (no reference image involved).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt": map[string]any{
						"type":        "string",
						"description": "Detailed description of the desired image.",
					},
					"aspect_ratio": map[string]any{
						"type":        "string",
						"description": `Optional aspect ratio. One of: "1:1", "16:9", "9:16", "4:3", "3:4". Default "1:1".`,
					},
				},
				"required": []string{"prompt"},
			},
		},
	}
	handler := func(ctx context.Context, args string) (string, error) {
		var p struct {
			Prompt      string `json:"prompt"`
			AspectRatio string `json:"aspect_ratio"`
		}
		if err := json.Unmarshal([]byte(args), &p); err != nil {
			return "", fmt.Errorf("%s: %w", cfg.toolName, err)
		}
		if p.Prompt == "" {
			return jsonErr("prompt is required"), nil
		}
		channelID := ChannelFromContext(ctx)
		mc := MessageContextFrom(ctx)
		logf(cfg.logf, "[%s] tool %s: prompt=%q ar=%q",
			short(channelID), cfg.toolName, truncateOneLine(p.Prompt, 80), p.AspectRatio)

		resp, err := client.Generate(ctx, &images.GenerateRequest{
			Prompt:         p.Prompt,
			AspectRatio:    p.AspectRatio,
			ResponseFormat: "url",
		})
		if err != nil {
			logf(cfg.logf, "[%s] tool %s: error: %v", short(channelID), cfg.toolName, err)
			return jsonErr(err.Error()), nil
		}
		if len(resp.Data) == 0 || resp.Data[0].URL == "" {
			logf(cfg.logf, "[%s] tool %s: no image returned", short(channelID), cfg.toolName)
			return jsonErr("no image returned"), nil
		}
		url := resp.Data[0].URL
		postMedia(sender, cfg.logf, cfg.toolName, channelID, mc.UserID, url)
		out, _ := json.Marshal(imageOut{
			ImageURL:      url,
			RevisedPrompt: resp.Data[0].RevisedPrompt,
			Posted:        sender != nil && channelID != "",
		})
		return string(out), nil
	}
	return tool, handler
}

// imageOut is the JSON shape returned by GenerateImageTool /
// EditImageTool. A typed struct skips the per-call map[string]any
// allocation and the reflection-heavy encoding path it triggers
// .
type imageOut struct {
	ImageURL      string `json:"image_url"`
	RevisedPrompt string `json:"revised_prompt"`
	Posted        bool   `json:"posted"`
}

// ----------------------------------------------------------------------------
// edit_image
// ----------------------------------------------------------------------------

// EditImageTool returns a (chat.Tool, chat.Handler) pair for
// image-to-image generation. The handler resolves the reference image
// in this priority order:
//
// 1. image_url argument from the model (when LooksLikeURL approves it);
// 2. MessageContext.FallbackImage from ctx (the caller's per-turn
// "current msg → replied-to msg → stored most-recent" resolution).
//
// If neither is set the tool returns an error to the chat agent so the
// model can fall back to plain generate_image or ask the user.
func EditImageTool(client *images.Client, sender Sender, opts ...MediaToolOption) (chat.Tool, chat.Handler) {
	cfg := mergeMediaOptions("edit_image", opts)
	tool := chat.Tool{
		Type: "function",
		Function: chat.FunctionDef{
			Name:        cfg.toolName,
			Description: `Generate an image based on a reference. Use when the user asks for a "similar" image, a variation, an edit, or anything that should derive from an existing image. The bot uses the most recent image attached to the channel as the reference automatically, you do NOT need to pass image_url. Only pass image_url if the user supplied an explicit URL different from the most-recently-attached channel image.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt": map[string]any{
						"type":        "string",
						"description": "Description of the desired output image, including how it should relate to the reference.",
					},
					"image_url": map[string]any{
						"type":        "string",
						"description": "Optional. Only set this if the user supplied an explicit URL different from the most-recently-attached channel image.",
					},
					"aspect_ratio": map[string]any{
						"type":        "string",
						"description": `Optional aspect ratio. One of: "1:1", "16:9", "9:16", "4:3", "3:4".`,
					},
				},
				"required": []string{"prompt"},
			},
		},
	}
	handler := func(ctx context.Context, args string) (string, error) {
		var p struct {
			Prompt      string `json:"prompt"`
			ImageURL    string `json:"image_url"`
			AspectRatio string `json:"aspect_ratio"`
		}
		if err := json.Unmarshal([]byte(args), &p); err != nil {
			return "", fmt.Errorf("%s: %w", cfg.toolName, err)
		}
		if p.Prompt == "" {
			return jsonErr("prompt is required"), nil
		}
		if !LooksLikeURL(p.ImageURL) {
			p.ImageURL = ""
		}
		channelID := ChannelFromContext(ctx)
		mc := MessageContextFrom(ctx)
		if p.ImageURL == "" {
			p.ImageURL = mc.FallbackImage
		}
		if p.ImageURL == "" {
			return jsonErr("no reference image: ask the user to attach one, or call generate_image instead"), nil
		}
		logf(cfg.logf, "[%s] tool %s: prompt=%q ref=%s ar=%q",
			short(channelID), cfg.toolName, truncateOneLine(p.Prompt, 60),
			short(lastPathSegment(p.ImageURL)), p.AspectRatio)

		resp, err := client.Edit(ctx, &images.EditRequest{
			Prompt:         p.Prompt,
			Image:          &images.ImageSource{URL: p.ImageURL},
			AspectRatio:    p.AspectRatio,
			ResponseFormat: "url",
		})
		if err != nil {
			logf(cfg.logf, "[%s] tool %s: error: %v", short(channelID), cfg.toolName, err)
			return jsonErr(err.Error()), nil
		}
		if len(resp.Data) == 0 || resp.Data[0].URL == "" {
			logf(cfg.logf, "[%s] tool %s: no image returned", short(channelID), cfg.toolName)
			return jsonErr("no image returned"), nil
		}
		url := resp.Data[0].URL
		postMedia(sender, cfg.logf, cfg.toolName, channelID, mc.UserID, url)
		out, _ := json.Marshal(imageOut{
			ImageURL:      url,
			RevisedPrompt: resp.Data[0].RevisedPrompt,
			Posted:        sender != nil && channelID != "",
		})
		return string(out), nil
	}
	return tool, handler
}

// ----------------------------------------------------------------------------
// generate_video / extend_video
// ----------------------------------------------------------------------------

// VideoToolOption customises a video tool factory. It composes
// MediaToolOption (name, logger) with video-specific knobs.
type VideoToolOption func(*videoToolConfig)

type videoToolConfig struct {
	mediaToolConfig
	pollInterval    time.Duration
	pollTimeout     time.Duration
	imageFetchBytes int64
	imageFetchTO    time.Duration
}

// WithVideoToolName overrides the advertised tool name. Defaults
// "generate_video" / "extend_video".
func WithVideoToolName(name string) VideoToolOption {
	return func(c *videoToolConfig) { c.toolName = name }
}

// WithVideoToolLogger plumbs an optional logger through the handler.
func WithVideoToolLogger(logf func(format string, args ...any)) VideoToolOption {
	return func(c *videoToolConfig) { c.logf = logf }
}

// WithVideoPollInterval sets how often the background goroutine polls
// xAI for video completion. Defaults 5s.
func WithVideoPollInterval(d time.Duration) VideoToolOption {
	return func(c *videoToolConfig) { c.pollInterval = d }
}

// WithVideoPollTimeout caps the background poll. After this elapses
// the helper posts a "giving up" message and stops; the underlying
// request keeps running on xAI's side. Defaults 15 minutes.
func WithVideoPollTimeout(d time.Duration) VideoToolOption {
	return func(c *videoToolConfig) { c.pollTimeout = d }
}

// WithImageFetchLimits configures the inline-as-data-URI download for
// image-to-video. Defaults: 30s timeout, 12 MiB cap.
func WithImageFetchLimits(timeout time.Duration, maxBytes int64) VideoToolOption {
	return func(c *videoToolConfig) {
		if timeout > 0 {
			c.imageFetchTO = timeout
		}
		if maxBytes > 0 {
			c.imageFetchBytes = maxBytes
		}
	}
}

func mergeVideoOptions(defaultName string, opts []VideoToolOption) *videoToolConfig {
	c := &videoToolConfig{
		mediaToolConfig: mediaToolConfig{toolName: defaultName},
		pollInterval:    5 * time.Second,
		pollTimeout:     15 * time.Minute,
		imageFetchBytes: 12 * 1024 * 1024,
		imageFetchTO:    30 * time.Second,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// GenerateVideoTool returns a (chat.Tool, chat.Handler) pair for video
// generation (text-to-video and image-to-video). The handler:
//
// - parses the model's args (prompt, optional duration_seconds /
// aspect_ratio, optional use_recent_image flag);
// - for image-to-video, downloads the reference image and inlines it
// as a base64 data URI before calling videos.Client.Generate. xAI's
// video service can't reliably fetch some external image hosts
// (notably Discord's CDN); inlining bytes sidesteps the cross-
// service fetcher entirely;
// - submits the request, returns request_id + status="generating" to
// the chat agent, and forks a background goroutine to poll for
// completion;
// - on completion, posts the resulting URL with the user's @-mention to the channel via
// sender, and (when memory != nil) records the URL so a later
// extend_video call can find it without the model copying URLs.
//
// memory may be nil, the tool just won't track recent URLs in that
// case.
func GenerateVideoTool(client *videos.Client, sender Sender, memory *URLRing, opts ...VideoToolOption) (chat.Tool, chat.Handler) {
	cfg := mergeVideoOptions("generate_video", opts)
	tool := chat.Tool{
		Type: "function",
		Function: chat.FunctionDef{
			Name:        cfg.toolName,
			Description: `Start generating a video. Async, the call returns immediately with status "generating"; the bot will post the finished video to the channel when it's ready (typically 1-5 minutes). For image-to-video, set use_recent_image=true and the bot uses the most recent image attached to the channel automatically, you do NOT need to pass image_url. After this tool returns, tell the user generation is underway and STOP calling this tool.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt": map[string]any{
						"type":        "string",
						"description": "Description of the video content and motion.",
					},
					"use_recent_image": map[string]any{
						"type":        "boolean",
						"description": "Set true for image-to-video using the most recent attached image. Set false (or omit) for text-only video.",
					},
					"image_url": map[string]any{
						"type":        "string",
						"description": "Optional. Only set this if the user supplied an explicit URL. Otherwise prefer use_recent_image=true.",
					},
					"duration_seconds": map[string]any{
						"type":        "integer",
						"description": "Optional duration in seconds (1-15). Defaults to the API default if omitted.",
					},
					"aspect_ratio": map[string]any{
						"type":        "string",
						"description": `Optional aspect ratio. One of: "16:9", "9:16", "1:1".`,
					},
				},
				"required": []string{"prompt"},
			},
		},
	}
	handler := func(ctx context.Context, args string) (string, error) {
		var p struct {
			Prompt         string `json:"prompt"`
			ImageURL       string `json:"image_url"`
			UseRecentImage bool   `json:"use_recent_image"`
			Duration       int    `json:"duration_seconds"`
			AspectRatio    string `json:"aspect_ratio"`
		}
		if err := json.Unmarshal([]byte(args), &p); err != nil {
			return "", fmt.Errorf("%s: %w", cfg.toolName, err)
		}
		if p.Prompt == "" {
			return jsonErr("prompt is required"), nil
		}
		if !LooksLikeURL(p.ImageURL) {
			p.ImageURL = ""
		}
		channelID := ChannelFromContext(ctx)
		mc := MessageContextFrom(ctx)
		if p.ImageURL == "" && p.UseRecentImage {
			p.ImageURL = mc.FallbackImage
		}

		req := &videos.GenerateRequest{
			Prompt:      p.Prompt,
			AspectRatio: p.AspectRatio,
		}
		mode := "text-to-video"
		if p.ImageURL != "" {
			logf(cfg.logf, "[%s] tool %s: fetching image for inline encoding (%s)",
				short(channelID), cfg.toolName, short(lastPathSegment(p.ImageURL)))
			dataURI, err := FetchAsDataURI(ctx, p.ImageURL,
				WithFetchTimeout(cfg.imageFetchTO),
				WithFetchMaxBytes(cfg.imageFetchBytes))
			if err != nil {
				logf(cfg.logf, "[%s] tool %s: image fetch failed: %v", short(channelID), cfg.toolName, err)
				return jsonErr("could not fetch reference image: " + err.Error()), nil
			}
			req.Image = &videos.VideoSource{URL: dataURI}
			mode = "image-to-video"
		}
		if p.Duration > 0 {
			req.Duration = &p.Duration
		}
		logf(cfg.logf, "[%s] tool %s: mode=%s prompt=%q dur=%d ar=%q",
			short(channelID), cfg.toolName, mode, truncateOneLine(p.Prompt, 60), p.Duration, p.AspectRatio)

		requestID, err := client.Generate(ctx, req)
		if err != nil {
			logf(cfg.logf, "[%s] tool %s: submit error: %v", short(channelID), cfg.toolName, err)
			return jsonErr(err.Error()), nil
		}
		logf(cfg.logf, "[%s] tool %s: submitted request_id=%s", short(channelID), cfg.toolName, requestID)
		go pollVideoAndPost(client, sender, memory, cfg, channelID, mc.UserID, requestID)

		out, _ := json.Marshal(videoSubmitOut{
			RequestID: requestID,
			Status:    "generating",
			Mode:      mode,
			Note:      "the video will be posted to the channel when it's ready (typically 1-5 minutes)",
		})
		return string(out), nil
	}
	return tool, handler
}

// videoSubmitOut is the JSON shape returned by GenerateVideoTool and
// ExtendVideoTool. Mode is empty for ExtendVideoTool. Typed struct
// avoids the per-call map[string]any allocation.
type videoSubmitOut struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	Mode      string `json:"mode,omitempty"`
	Note      string `json:"note"`
}

// ExtendVideoTool returns a (chat.Tool, chat.Handler) pair for video
// extension. Resolves the reference video URL in priority order:
//
// 1. video_url argument from the model (LooksLikeURL gate);
// 2. MessageContext.FallbackVideo from ctx (caller's per-turn
// resolution from current msg / replied-to msg);
// 3. memory.Recent(channelID), most-recent bot-generated video URL,
// written by GenerateVideoTool's poll routine.
//
// All three may be nil/empty; in that case the tool returns an error
// so the model can ask the user. memory may be nil to disable the
// stored-most-recent fallback.
func ExtendVideoTool(client *videos.Client, sender Sender, memory *URLRing, opts ...VideoToolOption) (chat.Tool, chat.Handler) {
	cfg := mergeVideoOptions("extend_video", opts)
	tool := chat.Tool{
		Type: "function",
		Function: chat.FunctionDef{
			Name:        cfg.toolName,
			Description: `Extend an existing video with a continuation. Async, the call returns immediately; the bot posts the extended video when ready (1-5 minutes). The bot uses the most recent bot-generated video in this channel automatically, you do NOT need to pass video_url. After this tool returns, tell the user it's underway and STOP calling tools.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt": map[string]any{
						"type":        "string",
						"description": "Description of how the video should continue / what should happen next.",
					},
					"video_url": map[string]any{
						"type":        "string",
						"description": "Optional. Only set this if the user explicitly supplied a video URL. Otherwise the most-recent bot-generated channel video is used automatically.",
					},
					"duration_seconds": map[string]any{
						"type":        "integer",
						"description": "Optional duration in seconds (1-10). Defaults to the API default if omitted.",
					},
				},
				"required": []string{"prompt"},
			},
		},
	}
	handler := func(ctx context.Context, args string) (string, error) {
		var p struct {
			Prompt   string `json:"prompt"`
			VideoURL string `json:"video_url"`
			Duration int    `json:"duration_seconds"`
		}
		if err := json.Unmarshal([]byte(args), &p); err != nil {
			return "", fmt.Errorf("%s: %w", cfg.toolName, err)
		}
		if p.Prompt == "" {
			return jsonErr("prompt is required"), nil
		}
		if !LooksLikeURL(p.VideoURL) {
			p.VideoURL = ""
		}
		channelID := ChannelFromContext(ctx)
		mc := MessageContextFrom(ctx)
		if p.VideoURL == "" {
			p.VideoURL = mc.FallbackVideo
		}
		if p.VideoURL == "" {
			p.VideoURL = memory.Recent(channelID)
		}
		if p.VideoURL == "" {
			return jsonErr("no video to extend: ask the user to share or generate one first"), nil
		}

		req := &videos.ExtendRequest{
			Prompt: p.Prompt,
			Video:  videos.VideoSource{URL: p.VideoURL},
		}
		if p.Duration > 0 {
			req.Duration = &p.Duration
		}
		logf(cfg.logf, "[%s] tool %s: prompt=%q ref=%s dur=%d",
			short(channelID), cfg.toolName, truncateOneLine(p.Prompt, 60),
			short(lastPathSegment(p.VideoURL)), p.Duration)

		requestID, err := client.Extend(ctx, req)
		if err != nil {
			logf(cfg.logf, "[%s] tool %s: submit error: %v", short(channelID), cfg.toolName, err)
			return jsonErr(err.Error()), nil
		}
		logf(cfg.logf, "[%s] tool %s: submitted request_id=%s", short(channelID), cfg.toolName, requestID)
		go pollVideoAndPost(client, sender, memory, cfg, channelID, mc.UserID, requestID)

		out, _ := json.Marshal(videoSubmitOut{
			RequestID: requestID,
			Status:    "generating",
			Note:      "the extended video will be posted to the channel when it's ready (typically 1-5 minutes)",
		})
		return string(out), nil
	}
	return tool, handler
}

// pollVideoAndPost is the shared async poll loop used by both
// GenerateVideoTool and ExtendVideoTool. It logs every status
// transition (so silent failures show up in the host's logs), posts a
// success/failure/timeout message to the channel via sender, and
// records successful URLs into memory for future extend_video lookups.
//
// channelID may be "" (sender post is skipped); userID may be "" (no
// @-mention).
func pollVideoAndPost(client *videos.Client, sender Sender, memory *URLRing, cfg *videoToolConfig, channelID, userID, requestID string) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.pollTimeout)
	defer cancel()

	rid := short(requestID)
	cid := short(channelID)
	logf(cfg.logf, "[%s] %s %s: poll started", cid, cfg.toolName, rid)

	deadline := time.Now().Add(cfg.pollTimeout)
	var lastStatus string
	var lastProgress int

	for {
		result, err := client.GetResult(ctx, requestID)
		if err != nil {
			// Distinguish "user cancelled / process shutting down" from
			// "the underlying job actually failed". On
			// cancellation we silently return; the user knows they
			// cancelled, no Discord post needed.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				logf(cfg.logf, "[%s] %s %s: poll cancelled: %v", cid, cfg.toolName, rid, err)
				return
			}
			logf(cfg.logf, "[%s] %s %s: poll error: %v", cid, cfg.toolName, rid, err)
			postChannel(sender, channelID, fmt.Sprintf("%s%s failed (request_id=`%s`): %v",
				Mention(userID), cfg.toolName, requestID, err))
			return
		}
		if result.Status != lastStatus || result.Progress != lastProgress {
			logf(cfg.logf, "[%s] %s %s: status=%s progress=%d%%",
				cid, cfg.toolName, rid, result.Status, result.Progress)
			lastStatus = result.Status
			lastProgress = result.Progress
		}

		switch result.Status {
		case "done":
			if result.Video == nil || result.Video.URL == "" {
				logf(cfg.logf, "[%s] %s %s: done but no URL returned", cid, cfg.toolName, rid)
				postChannel(sender, channelID, fmt.Sprintf("%s%s done but no URL returned (request_id=`%s`).",
					Mention(userID), cfg.toolName, requestID))
				return
			}
			logf(cfg.logf, "[%s] %s %s: success url=%s", cid, cfg.toolName, rid, result.Video.URL)
			memory.Add(channelID, result.Video.URL)
			postChannel(sender, channelID, fmt.Sprintf("%s🎬 %s", Mention(userID), result.Video.URL))
			return
		case "failed":
			msg := fmt.Sprintf("%s%s failed (request_id=`%s`)", Mention(userID), cfg.toolName, requestID)
			errMsg := ""
			if result.Error != nil {
				errMsg = result.Error.Message
				msg += ": " + errMsg
			}
			logf(cfg.logf, "[%s] %s %s: failed: %s", cid, cfg.toolName, rid, errMsg)
			postChannel(sender, channelID, msg)
			return
		}

		if time.Now().After(deadline) {
			logf(cfg.logf, "[%s] %s %s: poll timeout after %s", cid, cfg.toolName, rid, cfg.pollTimeout)
			postChannel(sender, channelID, fmt.Sprintf(
				"%s%s still pending after %s (request_id=`%s`); giving up. The job may still complete on xAI's side.",
				Mention(userID), cfg.toolName, cfg.pollTimeout, requestID))
			return
		}
		t := time.NewTimer(cfg.pollInterval)
		select {
		case <-ctx.Done():
			t.Stop()
			logf(cfg.logf, "[%s] %s %s: ctx cancelled: %v", cid, cfg.toolName, rid, ctx.Err())
			return
		case <-t.C:
		}
	}
}

// ----------------------------------------------------------------------------
// FetchAsDataURI, small helper for cross-service fetcher quirks.
//
// Defends against SSRF: the URL is validated at parse
// time, every DNS-resolved address is checked against private/loopback/
// link-local ranges, the dial-time check defeats DNS rebinding, and
// every redirect is re-validated.
// ----------------------------------------------------------------------------

// FetchOption customises FetchAsDataURI.
type FetchOption func(*fetchConfig)

type fetchConfig struct {
	timeout      time.Duration
	maxBytes     int64
	client       *http.Client
	allowHTTP    bool
	allowPrivate bool
	allowList    map[string]struct{}
}

// WithFetchTimeout caps how long FetchAsDataURI will wait. Defaults
// 30s. Values <= 0 are ignored.
func WithFetchTimeout(d time.Duration) FetchOption {
	return func(c *fetchConfig) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// WithFetchMaxBytes caps the size of the body FetchAsDataURI will
// inline. Bytes past this trigger an error. Defaults 12 MiB.
// Values <= 0 are ignored.
func WithFetchMaxBytes(n int64) FetchOption {
	return func(c *fetchConfig) {
		if n > 0 {
			c.maxBytes = n
		}
	}
}

// WithFetchHTTPClient overrides the http.Client. When set, the SDK's
// SSRF protections (URL scheme + IP validation, redirect re-check,
// dial-time IP guard) are NOT applied to the supplied client. Use only
// if your client implements equivalent protections.
func WithFetchHTTPClient(client *http.Client) FetchOption {
	return func(c *fetchConfig) { c.client = client }
}

// WithFetchAllowHTTP permits http:// URLs. By default only https:// is
// accepted; calls with http:// URLs are rejected without dialing.
// Enable for testing against local fixtures or for an internal allow-
// listed host that does not speak TLS.
func WithFetchAllowHTTP() FetchOption {
	return func(c *fetchConfig) { c.allowHTTP = true }
}

// WithFetchAllowPrivateAddresses disables the IP-range check that
// rejects loopback, RFC 1918, link-local, and similar non-public
// destinations. Enable only for tests or for a deliberately confined
// internal-network use case. Combine with WithFetchAllowList to
// restrict which hosts may resolve to private space.
func WithFetchAllowPrivateAddresses() FetchOption {
	return func(c *fetchConfig) { c.allowPrivate = true }
}

// WithFetchAllowList restricts FetchAsDataURI to the given set of
// hostnames. URLs targeting any other host are rejected before
// resolution. Pass an empty list to disable the allow-list (default).
func WithFetchAllowList(hosts ...string) FetchOption {
	return func(c *fetchConfig) {
		if len(hosts) == 0 {
			c.allowList = nil
			return
		}
		set := make(map[string]struct{}, len(hosts))
		for _, h := range hosts {
			set[strings.ToLower(strings.TrimSpace(h))] = struct{}{}
		}
		c.allowList = set
	}
}

// FetchAsDataURI downloads url and returns it as a
// "data:<mime>;base64,<bytes>" URI.
//
// Useful when one xAI service can't fetch from another host (the
// classic case: video generation fails to fetch reference images from
// Discord's CDN). Inlining the bytes as a data URI sidesteps the
// cross-service fetcher entirely at the cost of a 33% size inflation
// from base64 encoding. Use the size cap (default 12 MiB) to keep
// request bodies sane.
//
// Security: the URL is validated against an SSRF guard before any
// network activity. By default only https:// is accepted, and the
// resolved host must be a public IP. The dial-time check re-resolves
// to defeat DNS rebinding, and HTTP redirects are re-validated. Use
// WithFetchAllowHTTP, WithFetchAllowPrivateAddresses, or
// WithFetchAllowList to relax the guard explicitly.
func FetchAsDataURI(ctx context.Context, rawURL string, opts ...FetchOption) (string, error) {
	c := &fetchConfig{
		timeout:  30 * time.Second,
		maxBytes: 12 * 1024 * 1024,
	}
	for _, o := range opts {
		o(c)
	}

	if _, err := validateFetchURL(rawURL, c); err != nil {
		return "", fmt.Errorf("FetchAsDataURI: %w", err)
	}
	if c.client == nil {
		c.client = newSafeFetchClient(c)
	}

	fetchCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	if int64(len(body)) > c.maxBytes {
		return "", fmt.Errorf("body too large (>%d bytes)", c.maxBytes)
	}

	mime := resp.Header.Get("Content-Type")
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
	mime = strings.TrimSpace(mime)
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(body), nil
}

// validateFetchURL parses raw and applies the SSRF guard. Returns the
// parsed URL on success. The guard:
//
// - rejects schemes other than https (or http when allowHTTP is set)
// - rejects empty hostnames
// - rejects hosts not on the allow-list (when one is configured)
// - resolves the host and rejects any non-public address (unless
// allowPrivate is set)
func validateFetchURL(raw string, c *fetchConfig) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	switch u.Scheme {
	case "https":
		// permitted
	case "http":
		if !c.allowHTTP {
			return nil, fmt.Errorf("scheme not allowed: http (use WithFetchAllowHTTP to opt in)")
		}
	default:
		return nil, fmt.Errorf("scheme not allowed: %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("missing host in url")
	}
	if c.allowList != nil {
		if _, ok := c.allowList[strings.ToLower(host)]; !ok {
			return nil, fmt.Errorf("host %q not on allow-list", host)
		}
	}
	if !c.allowPrivate {
		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, fmt.Errorf("resolve host: %w", err)
		}
		for _, ip := range ips {
			if !isPublicIP(ip) {
				return nil, fmt.Errorf("non-public destination: %s", ip)
			}
		}
	}
	return u, nil
}

// isPublicIP reports whether ip is routable on the public internet.
func isPublicIP(ip net.IP) bool {
	return !ip.IsLoopback() &&
		!ip.IsPrivate() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() &&
		!ip.IsInterfaceLocalMulticast() &&
		!ip.IsMulticast() &&
		!ip.IsUnspecified()
}

// newSafeFetchClient builds an http.Client whose redirects re-run the
// URL validation and whose DialContext re-checks the resolved address.
// The dial-time check defeats DNS rebinding (host resolves to a public
// IP at validate time but to loopback at dial time).
func newSafeFetchClient(c *fetchConfig) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			_, err := validateFetchURL(req.URL.String(), c)
			return err
		},
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, _, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
				if err != nil {
					return nil, err
				}
				for _, ip := range ips {
					if !c.allowPrivate && !isPublicIP(ip) {
						return nil, fmt.Errorf("non-public dial target: %s", ip)
					}
				}
				return dialer.DialContext(ctx, network, addr)
			},
		},
	}
}

// ----------------------------------------------------------------------------
// internal helpers shared across this file and server_tools.go
// ----------------------------------------------------------------------------

// logf is a nil-safe printf shim.
func logf(f func(format string, args ...any), format string, args ...any) {
	if f != nil {
		f(format, args...)
	}
}

// postChannel sends content via sender (no-op if sender is nil or
// channelID is empty).
func postChannel(sender Sender, channelID, content string) {
	if sender == nil || channelID == "" {
		return
	}
	_ = sender.Send(channelID, content)
}

// postMedia is the standard "post a media URL with @mention" helper.
// Logs send failures via logf if non-nil.
func postMedia(sender Sender, logger func(string, ...any), tool, channelID, userID, url string) {
	if sender == nil || channelID == "" {
		return
	}
	logf(logger, "[%s] tool %s: posted url=%s", short(channelID), tool, url)
	if err := sender.Send(channelID, Mention(userID)+url); err != nil {
		logf(logger, "[%s] tool %s: send error: %v", short(channelID), tool, err)
	}
}

// lastPathSegment returns the last "/"-delimited segment of a URL
// path, stripping query string. Used in log lines to keep them
// readable when URLs carry signed-token query strings.
func lastPathSegment(u string) string {
	if i := strings.IndexByte(u, '?'); i >= 0 {
		u = u[:i]
	}
	if i := strings.LastIndexByte(u, '/'); i >= 0 {
		return u[i+1:]
	}
	return u
}
